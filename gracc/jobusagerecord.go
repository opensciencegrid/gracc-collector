package gracc

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

type recordIdentity struct {
	RecordId   string    `xml:"recordId,attr"`
	CreateTime time.Time `xml:"createTime,attr"`
}

func (i *recordIdentity) flatten() map[string]interface{} {
	var r = make(map[string]interface{})
	if i.RecordId != "" {
		r["RecordId"] = i.RecordId
	}
	if !i.CreateTime.IsZero() {
		r["CreateTime"] = i.CreateTime.Format(time.RFC3339)
	}
	return r
}

type jobIdentity struct {
	GlobalJobId string
	LocalJobId  string
	ProcessId   []string
}

func (i *jobIdentity) flatten() map[string]interface{} {
	var r = make(map[string]interface{})
	if i.GlobalJobId != "" {
		r["GlobalJobId"] = i.GlobalJobId
	}
	if i.LocalJobId != "" {
		r["LocalJobId"] = i.LocalJobId
	}
	if len(i.ProcessId) == 1 {
		r["ProcessId"] = i.ProcessId[0]
	} else if len(i.ProcessId) > 1 {
		for n, v := range i.ProcessId {
			k := fmt.Sprintf("ProcessId%d", n)
			r[k] = v
		}
	}
	return r
}

type userIdentity struct {
	GlobalUsername   string
	LocalUserId      string
	VOName           string
	ReportableVOName string
	CommonName       string
	DN               string
}

func (i *userIdentity) flatten() map[string]interface{} {
	var r = make(map[string]interface{})
	if i.GlobalUsername != "" {
		r["GlobalUsername"] = i.GlobalUsername
	}
	if i.LocalUserId != "" {
		r["LocalUserId"] = i.LocalUserId
	}
	if i.VOName != "" {
		r["VOName"] = i.VOName
	}
	if i.ReportableVOName != "" {
		r["ReportableVOName"] = i.ReportableVOName
	}
	if i.CommonName != "" {
		r["CommonName"] = i.CommonName
	}
	if i.DN != "" {
		r["DN"] = i.DN
	}
	return r
}

type cpuDuration struct {
	UsageType   string `xml:"usageType,attr"`
	Description string `xml:"description,attr"`
	Value       string `xml:",chardata"`
}

type wallDuration struct {
	Description string `xml:"description,attr"`
	Value       string `xml:",chardata"`
}

type resource struct {
	XMLName     xml.Name
	Value       string `xml:",chardata"`
	Description string `xml:"description,attr"`
	Unit        string `xml:"unit,attr,omitempty"`
	PhaseUnit   string `xml:"phaseUnit,attr,omitempty"`
	StorageUnit string `xml:"storageUnit,attr,omitempty"`
}

func (r *resource) flatten() map[string]interface{} {
	k := "unknown"
	if r.Description != "" {
		k = strings.Map(mapForKey, r.Description)
	}
	var rr = map[string]interface{}{
		k: r.Value,
	}
	if r.Unit != "" {
		rr[k+"_unit"] = r.Unit
	}
	if r.PhaseUnit != "" {
		rr[k+"_phaseUnit"] = convertDurationToSeconds(r.PhaseUnit)
	}
	if r.StorageUnit != "" {
		rr[k+"_storageUnit"] = r.StorageUnit
	}
	return rr
}

// JobUsageRecord is a flexible container for batch job accounting records.
type JobUsageRecord struct {
	XMLName            xml.Name
	RecordIdentity     recordIdentity `xml:",omitempty"`
	JobIdentity        jobIdentity    `xml:",omitempty"`
	UserIdentity       userIdentity   `xml:",omitempty"`
	WallDuration       wallDuration   `xml:",omitempty"`
	CpuDuration        []cpuDuration  `xml:",omitempty"`
	StartTime          time.Time      `xml:",omitempty"`
	EndTime            time.Time      `xml:",omitempty"`
	QueueTime          time.Time      `xml:",omitempty"`
	TimeDuration       []timeDuration `xml:",omitempty"`
	TimeInstant        []timeInstant  `xml:",omitempty"`
	Resource           []resource     `xml:",omitempty"`
	ConsumableResource []resource     `xml:",omitempty"`
	PhaseResource      []resource     `xml:",omitempty"`
	VolumeResource     []resource     `xml:",omitempty"`
	Origin             origin         `xml:",omitempty"`
	Fields             []field        `xml:",any"`
	RawXML             []byte         `xml:",innerxml"`
}

// ParseXML attempts to unmarshal the XML in xb into a JobUsageRecord.
func (jur *JobUsageRecord) ParseXML(xb []byte) error {
	if err := xml.Unmarshal(xb, jur); err != nil {
		return err
	}
	return nil
}

// Id returns an identification string for the record.
func (jur *JobUsageRecord) Id() string {
	return jur.RecordIdentity.RecordId
}

// Type returns the type of the record.
func (jur *JobUsageRecord) Type() string {
	return jur.XMLName.Local
}

// Raw returns the unaltered source of the record.
func (jur *JobUsageRecord) Raw() []byte {
	s := "<" + jur.XMLName.Local + ">" + string(jur.RawXML) + "</" + jur.XMLName.Local + ">"
	return []byte(s)
}

// ToJSON returns a JSON encoding of the Record, with certain elements
// transformed to fit the GRACC Raw Record schema.
// Indent specifies the string to use for each indentation level,
// if empty no indentation or pretty-printing is performed.
func (jur *JobUsageRecord) ToJSON(indent string) ([]byte, error) {
	var r = make(map[string]interface{})

	r["type"] = "JobUsageRecord"

	// Flatten identity blocks
	for k, v := range jur.RecordIdentity.flatten() {
		r[k] = v
	}
	for k, v := range jur.JobIdentity.flatten() {
		r[k] = v
	}
	for k, v := range jur.UserIdentity.flatten() {
		r[k] = v
	}

	// Standard time instants
	if !jur.StartTime.IsZero() {
		r["StartTime"] = jur.StartTime.Format(time.RFC3339)
	}
	if !jur.EndTime.IsZero() {
		r["EndTime"] = jur.EndTime.Format(time.RFC3339)
	}
	if !jur.QueueTime.IsZero() {
		r["QueueTime"] = jur.QueueTime.Format(time.RFC3339)
	}

	// Standard durations
	var totalCpu float64
	for _, c := range jur.CpuDuration {
		secs := convertDurationToSeconds(c.Value)
		if secs > 0 {
			totalCpu += secs
		}
		if c.UsageType != "" {
			r["CpuDuration_"+c.UsageType] = secs
		}
		if c.Description != "" {
			r["CpuDuration_"+c.UsageType+"_description"] = c.Description
		}
	}
	r["CpuDuration"] = totalCpu
	r["WallDuration"] = convertDurationToSeconds(jur.WallDuration.Value)
	if jur.WallDuration.Description != "" {
		r["WallDuration_description"] = jur.WallDuration.Description
	}

	// flatten resources
	if len(jur.Resource) > 0 {
		for _, resa := range [][]resource{jur.Resource,
			jur.ConsumableResource,
			jur.PhaseResource,
			jur.VolumeResource,
		} {
			for _, res := range resa {
				for k, v := range res.flatten() {
					r["Resource_"+k] = v
				}
			}
		}
	}
	// Rename/Add ResourceType
	switch r["Resource_ResourceType"] {
	case nil, "", "Batch":
		r["ResourceType"] = "Batch"
	case "BatchPilot":
		r["ResourceType"] = "Payload"
	default:
		r["ResourceType"] = r["Resource_ResourceType"]
	}
	delete(r, "Resource_ResourceType")

	// time durations and instants
	for _, td := range jur.TimeDuration {
		for k, v := range td.flatten() {
			r["TimeDuration_"+k] = v
		}
	}
	for _, td := range jur.TimeInstant {
		for k, v := range td.flatten() {
			r["TimeInstant_"+k] = v
		}
	}

	// flatten other fields
	for _, f := range jur.Fields {
		for k, v := range f.flatten() {
			r[k] = v
		}
	}

	// origin
	for k, v := range jur.Origin.flatten() {
		r[k] = v
	}

	// add XML
	r["RawXML"] = string(jur.Raw())

	if indent != "" {
		return json.MarshalIndent(r, "", indent)
	}
	return json.Marshal(r)
}
