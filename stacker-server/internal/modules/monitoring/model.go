package monitoring

// Summary is the latest useful reading for a node. Nil means the exporter has
// not produced that metric yet, rather than pretending a missing reading is 0.
type Summary struct {
	Available bool     `json:"available"`
	Message   string   `json:"message,omitempty"`
	CPU       *float64 `json:"cpu,omitempty"`
	Memory    *float64 `json:"memory,omitempty"`
	Disk      *float64 `json:"disk,omitempty"`
	Load1     *float64 `json:"load1,omitempty"`
	Uptime    *float64 `json:"uptime,omitempty"`
}

// Point is one timestamp/value pair sent to the chart. Keeping the response
// small and presentation-neutral lets the SPA use a native SVG chart.
type Point struct {
	At    int64   `json:"at"`
	Value float64 `json:"value"`
}

type Series struct {
	Name   string  `json:"name"`
	Unit   string  `json:"unit"`
	Points []Point `json:"points"`
}

type Dashboard struct {
	Range           string   `json:"range"`
	CPU             []Series `json:"cpu"`
	Memory          []Series `json:"memory"`
	Disk            []Series `json:"disk"`
	DiskIO          []Series `json:"diskIo"`
	Network         []Series `json:"network"`
	Containers      []Series `json:"containers"`
	ContainerMemory []Series `json:"containerMemory"`
}
