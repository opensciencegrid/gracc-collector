package main

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	log "github.com/Sirupsen/logrus"
	"github.com/opensciencegrid/gracc-collector/gracc"
	"github.com/prometheus/client_golang/prometheus"
)

type CollectorStats struct {
	Records       uint64
	RecordErrors  uint64
	Requests      uint64
	RequestErrors uint64
}

type Event int

const (
	GOT_RECORD Event = iota
	RECORD_ERROR
	GOT_REQUEST
	REQUEST_ERROR
)

type GraccCollector struct {
	Config *CollectorConfig
	Output *AMQPOutput
	Stats  CollectorStats
	m      sync.Mutex

	Events chan Event

	RecordCountDesc       *prometheus.Desc
	RecordErrorCountDesc  *prometheus.Desc
	RequestCountDesc      *prometheus.Desc
	RequestErrorCountDesc *prometheus.Desc
}

// NewCollector initializes and returns a new Gracc collector.
func NewCollector(conf *CollectorConfig) (*GraccCollector, error) {
	var g GraccCollector
	g.Config = conf

	g.Events = make(chan Event)
	go g.LogEvents()

	if o, err := InitAMQP(conf.AMQP); err != nil {
		return nil, err
	} else {
		g.Output = o
	}

	g.RecordCountDesc = prometheus.NewDesc(
		"gracc_records_total",
		"Number of records processed.",
		nil,
		nil,
	)
	g.RecordErrorCountDesc = prometheus.NewDesc(
		"gracc_record_errors_total",
		"Number of records with errors.",
		nil,
		nil,
	)
	g.RequestCountDesc = prometheus.NewDesc(
		"gracc_requests_total",
		"Number of requests received.",
		nil,
		nil,
	)
	g.RequestErrorCountDesc = prometheus.NewDesc(
		"gracc_request_errors_total",
		"Number of requests with errors.",
		nil,
		nil,
	)

	return &g, nil
}

func (g *GraccCollector) LogEvents() {
	for e := range g.Events {
		g.m.Lock()
		switch e {
		case GOT_RECORD:
			g.Stats.Records++
		case RECORD_ERROR:
			g.Stats.RecordErrors++
		case GOT_REQUEST:
			g.Stats.Requests++
		case REQUEST_ERROR:
			g.Stats.RequestErrors++
		}
		g.m.Unlock()
	}
}

func (g *GraccCollector) ServeStats(w http.ResponseWriter, r *http.Request) {
	enc := json.NewEncoder(w)
	g.m.Lock()
	stats := g.Stats
	g.m.Unlock()
	if err := enc.Encode(stats); err != nil {
		log.WithField("err", err).Error("error encoding stats")
		http.Error(w, "error writing stats", http.StatusInternalServerError)
	}
}

func (g *GraccCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- g.RecordCountDesc
	ch <- g.RecordErrorCountDesc
	ch <- g.RequestCountDesc
	ch <- g.RequestErrorCountDesc
}

func (g *GraccCollector) Collect(ch chan<- prometheus.Metric) {
	g.m.Lock()
	ch <- prometheus.MustNewConstMetric(
		g.RecordCountDesc,
		prometheus.CounterValue,
		float64(g.Stats.Records),
	)
	ch <- prometheus.MustNewConstMetric(
		g.RecordErrorCountDesc,
		prometheus.CounterValue,
		float64(g.Stats.RecordErrors),
	)
	ch <- prometheus.MustNewConstMetric(
		g.RequestCountDesc,
		prometheus.CounterValue,
		float64(g.Stats.Requests),
	)
	ch <- prometheus.MustNewConstMetric(
		g.RequestErrorCountDesc,
		prometheus.CounterValue,
		float64(g.Stats.RequestErrors),
	)
	g.m.Unlock()
}

// Request is a wrapper struct for passing around an HTTP request
// and associated response writer and metadata
type Request struct {
	w     http.ResponseWriter
	r     *http.Request
	log   *log.Entry
	start time.Time
}

func (g *GraccCollector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.Events <- GOT_REQUEST
	var remoteAddr string

	// Check for forwarded headers
	if remote := r.Header.Get("X-Real-IP"); remote != "" {
		remoteAddr = remote
	} else if remote := r.Header.Get("X-Forwarded-For"); remote != "" {
		remoteAddr = remote
	} else {
		remoteAddr = r.RemoteAddr
	}

	req := &Request{
		w: w,
		r: r,
		log: log.WithFields(log.Fields{
			"address":  remoteAddr,
			"length":   r.ContentLength,
			"agent":    r.UserAgent(),
			"url_path": r.URL.EscapedPath(),
		}),
		start: time.Now(),
	}
	r.ParseForm()
	if err := g.checkRequiredKeys(req, []string{"command"}); err != nil {
		g.Events <- REQUEST_ERROR
		g.handleError(req, err)
		return
	}
	command := r.FormValue("command")
	switch command {
	case "update":
		g.handleUpdate(req)
	case "multiupdate":
		g.handleMultiUpdate(req)
	default:
		g.Events <- REQUEST_ERROR
		g.handleError(req, NewRequestError("unknown command"))
	}
}

// handleMultiUpdate handles the typical request from a Gratia probe.
//
// Typical fields included in request:
//     command: update type (typ. "multiupdate")
//     arg1: record bundle XML
//     bundlesize: max number of records in bundle
//     from: the name of the sender
// Extra:
//     xmlfiles: number of records already passed to GratiaCore, still to be sent and still in individual xml files (i.e. number of gratia record in the outbox)
//     tarfiles: number of outstanding tar files
//     maxpendingfiles: 'current' number of files in a new tar file (i.e. an estimate of the number of individual records per tar file).
//     backlog: estimated amount of data to be processed by the probe
func (g *GraccCollector) handleMultiUpdate(req *Request) {
	if err := g.checkRequiredKeys(req, []string{"arg1", "from"}); err != nil {
		g.Events <- REQUEST_ERROR
		g.handleError(req, err)
		return
	}
	updateLogger := log.WithFields(log.Fields{
		"from": req.r.FormValue("from"),
	})
	var bun gracc.RecordBundle
	if err := xml.Unmarshal([]byte(req.r.FormValue("arg1")), &bun); err != nil {
		g.Events <- REQUEST_ERROR
		updateLogger.WithField("error", err).Error("error unmarshalling xml")
		g.handleError(req, NewRequestError("error unmarshalling xml"))
		return
	}
	updateLogger.WithFields(log.Fields{
		"UsageRecord":          len(bun.UsageRecords),
		"JobUsageRecord":       len(bun.JobUsageRecords),
		"StorageElement":       len(bun.StorageElements),
		"StorageElementRecord": len(bun.StorageElementRecords),
		"Other":                len(bun.OtherRecords),
	}).Debug("processed XML record bundle")
	if err := g.sendBundle(&bun); err != nil {
		g.Events <- REQUEST_ERROR
		updateLogger.WithField("error", err).Error("error sending update")
		g.handleError(req, err)
		return
	}
	updateLogger.WithField("bundlesize", bun.RecordCount()).Info("received multiupdate")
	g.handleSuccess(req)
}

// handleUpdate handles the typical request from a Gratia collector.
func (g *GraccCollector) handleUpdate(req *Request) {
	if err := g.checkRequiredKeys(req, []string{"arg1", "from"}); err != nil {
		g.Events <- REQUEST_ERROR
		g.handleError(req, err)
		return
	}
	updateLogger := log.WithFields(log.Fields{
		"from": req.r.FormValue("from"),
	})
	if req.r.FormValue("arg1") == "xxx" {
		updateLogger.Info("received ping")
		g.handleSuccess(req)
		return
	}
	if err := g.checkRequiredKeys(req, []string{"bundlesize"}); err != nil {
		g.Events <- REQUEST_ERROR
		g.handleError(req, err)
		return
	}
	bundlesize, err := strconv.Atoi(req.r.FormValue("bundlesize"))
	if err != nil {
		g.Events <- REQUEST_ERROR
		g.handleError(req, NewRequestError("error interpreting bundlesize"))
		return
	}
	bun, err := g.processBundle(req.r.FormValue("arg1"))
	if err != nil {
		g.Events <- REQUEST_ERROR
		updateLogger.WithField("error", err).Error("error processing bundle")
		g.handleError(req, err)
		return
	}
	if n := bun.RecordCount(); n != bundlesize {
		g.Events <- REQUEST_ERROR
		g.handleError(req, NewRequestError(fmt.Sprintf("number of records in bundle (%d) different than expected (%d)", n, bundlesize)))
		return
	}
	if err := g.sendBundle(bun); err != nil {
		g.Events <- REQUEST_ERROR
		g.handleError(req, err)
		return
	}
	updateLogger.WithField("bundlesize", req.r.FormValue("bundlesize")).Info("received update")
	g.handleSuccess(req)
}

func (g *GraccCollector) checkRequiredKeys(req *Request, keys []string) error {
	for _, k := range keys {
		if req.r.FormValue(k) == "" {
			return NewRequestError(fmt.Sprintf("missing key \"%s\"", k))
		}
	}
	return nil
}

// processBundle parses a replication bundle.
func (g *GraccCollector) processBundle(bundle string) (*gracc.RecordBundle, error) {
	var bun gracc.RecordBundle
	bs := bufio.NewScanner(strings.NewReader(bundle))
	bs.Buffer(make([]byte, g.Config.StartBufferSize), g.Config.MaxBufferSize)
	bs.Split(ScanBundle)
ScannerLoop:
	for bs.Scan() {
		tok := bs.Text()
		switch tok {
		case "":
			continue
		case "replication":
			parts := make(map[string]string, 3)
			for _, p := range []string{"rec", "raw", "extra"} {
				if bs.Scan() {
					parts[p] = bs.Text()
				} else {
					break ScannerLoop
				}
			}
			rec, err := gracc.ParseRecordXML([]byte(parts["rec"]))
			if err != nil {
				log.WithFields(log.Fields{
					"error": err,
					"rec":   parts["rec"],
					"raw":   parts["raw"],
					"extra": parts["extra"],
				}).Error("error processing record XML")
				return nil, NewRecordError("error processing replicated record")
			}
			bun.AddRecord(rec)
		}
	}
	// check for scanner errors
	if err := bs.Err(); err != nil {
		return nil, NewRecordError(fmt.Sprintf("error parsing bundle: %s", err))
	}
	return &bun, nil
}

// sendBundle publishes the records in RecordBundle bun to AMQP.
func (g *GraccCollector) sendBundle(bun *gracc.RecordBundle) error {
	// setup AMQP channel
	w, err := g.Output.NewWorker(bun.RecordCount())
	if err != nil {
		log.Error(err)
		return err
	}
	defer w.Close()

	for _, r := range bun.OtherRecords {
		g.Events <- GOT_RECORD
		g.Events <- RECORD_ERROR
		log.WithField("type", r.XMLName).Warning("bundle contains unrecognized record type; ignoring!")
	}

	npub := 0
	for rec := range bun.Records() {
		g.Events <- GOT_RECORD
		npub += 1
		if err := w.PublishRecord(rec); err != nil {
			g.Events <- RECORD_ERROR
			npub -= 1
			return err
		}
	}
	if npub > 0 {
		// wait for confirms that all records were received and routed
		if err := w.Wait(g.Config.TimeoutDuration); err != nil {
			return err
		}
	}
	return nil
}

func (g *GraccCollector) handleError(req *Request, err error) {
	var msg string
	var code int
	switch err.(type) {
	case AMQPError:
		code = 503
		msg = "Service unavailable right now"
	case RequestError:
		code = 400
		msg = fmt.Sprintf("Error handling request: %s", err)
	case RecordError:
		code = 400
		msg = fmt.Sprintf("Error processing record: %s", err)
	default:
		code = 500
		msg = "Internal server error"
	}
	req.log.WithFields(log.Fields{
		"response":      msg,
		"response-code": code,
		"error":         err,
		"response-time": time.Since(req.start).Nanoseconds(),
	}).Info("handled request")
	req.w.WriteHeader(code)
	fmt.Fprint(req.w, msg)
}

func (g *GraccCollector) handleSuccess(req *Request) {
	req.log.WithFields(log.Fields{
		"response":      "OK",
		"response-code": 200,
		"response-time": time.Since(req.start).Nanoseconds(),
	}).Info("handled request")
	fmt.Fprintf(req.w, "OK")
}

// ScanBundle is a split function for bufio.Scanner that splits the bundle
// at each pipe/bar character "|" that does not occur in a double-
// quote delimited string.
func ScanBundle(data []byte, atEOF bool) (advance int, token []byte, err error) {
	inString := false
	escape := false
	var stringDelim rune
	for width, i := 0, 0; i < len(data); i += width {
		var r rune
		r, width = utf8.DecodeRune(data[i:])
		switch r {
		case '|':
			if !inString {
				return i + width, data[0:i], nil
			}
		case '"':
			if inString && !escape && r == stringDelim {
				inString = false
			} else if !inString {
				inString = true
				stringDelim = r
			}
		}
		escape = (r == '\\' && !escape)
	}
	// If we're at EOF, we have a final, non-terminated bundle. Return it.
	if atEOF {
		return len(data), data, bufio.ErrFinalToken
	}
	// Request more data.
	return 0, nil, nil
}
