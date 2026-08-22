package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"stacker/internal/modules/node"
)

var ErrUnavailable = errors.New("monitoring is unavailable")

type nodeLookup interface {
	Get(id string) (node.Node, error)
}

type Service struct {
	nodes  nodeLookup
	url    string
	client *http.Client
}

func NewService(nodes nodeLookup, metricsURL string) *Service {
	return &Service{
		nodes:  nodes,
		url:    strings.TrimRight(metricsURL, "/"),
		client: &http.Client{Timeout: 12 * time.Second},
	}
}

func (s *Service) Summary(ctx context.Context, id string) (Summary, error) {
	// A dead monitoring service must never hold a node page open indefinitely.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	n, err := s.nodes.Get(id)
	if err != nil {
		return Summary{}, err
	}
	if err := s.ready(n); err != nil {
		return Summary{Available: false, Message: err.Error()}, nil
	}

	values := map[string]*float64{}
	for key, query := range summaryQueries(n.SwarmNodeID) {
		value, err := s.instant(ctx, query)
		if err != nil {
			return Summary{Available: false, Message: "Monitoring service is not reachable yet."}, nil
		}
		values[key] = value
	}
	return Summary{Available: true, CPU: values["cpu"], Memory: values["memory"], Disk: values["disk"], Load1: values["load1"], Uptime: values["uptime"]}, nil
}

func (s *Service) Dashboard(ctx context.Context, id, requestedRange string) (Dashboard, error) {
	// Every chart query shares one short deadline. The caller receives empty
	// series rather than waiting once per chart if VictoriaMetrics is down.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	n, err := s.nodes.Get(id)
	if err != nil {
		return Dashboard{}, err
	}
	rangeDuration, step, err := chartRange(requestedRange)
	if err != nil {
		return Dashboard{}, err
	}
	if err := s.ready(n); err != nil {
		return Dashboard{Range: requestedRange}, nil
	}

	end := time.Now()
	start := end.Add(-rangeDuration)
	query := func(name, unit, expression string) Series {
		points, err := s.rangeQuery(ctx, expression, start, end, step)
		if err != nil {
			return Series{Name: name, Unit: unit}
		}
		return Series{Name: name, Unit: unit, Points: points}
	}
	idLabel := promLabel(n.SwarmNodeID)
	return Dashboard{
		Range: requestedRange,
		CPU: []Series{query("CPU", "%", fmt.Sprintf(
			`100 - (avg(rate(node_cpu_seconds_total{mode="idle"}[5m]) and on(instance) node_uname_info{nodename=%s}) * 100)`, idLabel))},
		Memory: []Series{query("Memory", "%", fmt.Sprintf(
			`100 * (1 - ((node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) and on(instance) node_uname_info{nodename=%s}))`, idLabel))},
		Disk: []Series{query("Disk", "%", fmt.Sprintf(
			`100 * (1 - ((node_filesystem_avail_bytes{mountpoint="/",fstype!~"tmpfs|overlay"} / node_filesystem_size_bytes{mountpoint="/",fstype!~"tmpfs|overlay"}) and on(instance) node_uname_info{nodename=%s}))`, idLabel))},
		DiskIO: []Series{
			query("Read", "B/s", fmt.Sprintf(`sum(rate(node_disk_read_bytes_total{nodename=%s}[5m]))`, idLabel)),
			query("Write", "B/s", fmt.Sprintf(`sum(rate(node_disk_written_bytes_total{nodename=%s}[5m]))`, idLabel)),
		},
		Network: []Series{
			query("Receive", "B/s", fmt.Sprintf(`sum(rate(node_network_receive_bytes_total{nodename=%s,device!="lo"}[5m]))`, idLabel)),
			query("Transmit", "B/s", fmt.Sprintf(`sum(rate(node_network_transmit_bytes_total{nodename=%s,device!="lo"}[5m]))`, idLabel)),
		},
		Containers:      s.containerSeries(ctx, n.SwarmNodeID, start, end, step, false),
		ContainerMemory: s.containerSeries(ctx, n.SwarmNodeID, start, end, step, true),
	}, nil
}

func (s *Service) containerSeries(ctx context.Context, nodeID string, start, end time.Time, step time.Duration, memory bool) []Series {
	query := fmt.Sprintf(`sum by (name) (rate(container_cpu_usage_seconds_total{container_label_com_docker_swarm_node_id=%s,image!=""}[5m])) * 100`, promLabel(nodeID))
	unit := "%"
	if memory {
		query = fmt.Sprintf(`sum by (name) (container_memory_working_set_bytes{container_label_com_docker_swarm_node_id=%s,image!=""})`, promLabel(nodeID))
		unit = "B"
	}
	result, err := s.rangeMatrix(ctx, query, start, end, step)
	if err != nil {
		return nil
	}
	series := make([]Series, 0, len(result))
	for _, item := range result {
		name := item.Metric["name"]
		if name == "" {
			name = "container"
		}
		series = append(series, Series{Name: name, Unit: unit, Points: item.Points})
	}
	return series
}

func (s *Service) ready(n node.Node) error {
	if s.url == "" {
		return fmt.Errorf("monitoring is not installed for this environment")
	}
	if n.SwarmNodeID == "" {
		return fmt.Errorf("monitoring starts after this node joins the swarm")
	}
	return nil
}

func summaryQueries(nodeID string) map[string]string {
	id := promLabel(nodeID)
	return map[string]string{
		"cpu":    fmt.Sprintf(`100 - (avg(rate(node_cpu_seconds_total{mode="idle"}[5m]) and on(instance) node_uname_info{nodename=%s}) * 100)`, id),
		"memory": fmt.Sprintf(`100 * (1 - ((node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) and on(instance) node_uname_info{nodename=%s}))`, id),
		"disk":   fmt.Sprintf(`100 * (1 - ((node_filesystem_avail_bytes{mountpoint="/",fstype!~"tmpfs|overlay"} / node_filesystem_size_bytes{mountpoint="/",fstype!~"tmpfs|overlay"}) and on(instance) node_uname_info{nodename=%s}))`, id),
		"load1":  fmt.Sprintf(`node_load1 and on(instance) node_uname_info{nodename=%s}`, id),
		"uptime": fmt.Sprintf(`(time() - node_boot_time_seconds) and on(instance) node_uname_info{nodename=%s}`, id),
	}
}

func chartRange(value string) (time.Duration, time.Duration, error) {
	switch value {
	case "10m":
		return 10 * time.Minute, 10 * time.Second, nil
	case "1h":
		return time.Hour, 30 * time.Second, nil
	case "12h":
		return 12 * time.Hour, 2 * time.Minute, nil
	case "24h":
		return 24 * time.Hour, 5 * time.Minute, nil
	case "7d":
		return 7 * 24 * time.Hour, 30 * time.Minute, nil
	case "30d":
		return 30 * 24 * time.Hour, 2 * time.Hour, nil
	default:
		return 0, 0, fmt.Errorf("unsupported monitoring range")
	}
}

func promLabel(value string) string { return strconv.Quote(value) }

type promResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []promResult `json:"result"`
	} `json:"data"`
	Error string `json:"error"`
}
type promResult struct {
	Metric map[string]string `json:"metric"`
	Value  []any             `json:"value"`
	Values [][]any           `json:"values"`
	Points []Point
}

func (s *Service) instant(ctx context.Context, query string) (*float64, error) {
	result, err := s.request(ctx, "/api/v1/query", url.Values{"query": {query}})
	if err != nil || len(result) == 0 || len(result[0].Value) < 2 {
		return nil, err
	}
	value, err := number(result[0].Value[1])
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (s *Service) rangeQuery(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Point, error) {
	result, err := s.rangeMatrix(ctx, query, start, end, step)
	if err != nil || len(result) == 0 {
		return nil, err
	}
	return result[0].Points, nil
}

func (s *Service) rangeMatrix(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]promResult, error) {
	return s.request(ctx, "/api/v1/query_range", url.Values{
		"query": {query}, "start": {strconv.FormatInt(start.Unix(), 10)}, "end": {strconv.FormatInt(end.Unix(), 10)}, "step": {strconv.Itoa(int(step.Seconds()))},
	})
}

func (s *Service) request(ctx context.Context, path string, values url.Values) ([]promResult, error) {
	if s.url == "" {
		return nil, ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url+path+"?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	response, err := s.client.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		io.Copy(io.Discard, response.Body)
		return nil, ErrUnavailable
	}
	var decoded promResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&decoded); err != nil {
		return nil, err
	}
	if decoded.Status != "success" {
		return nil, fmt.Errorf("metrics query failed: %s", decoded.Error)
	}
	for i := range decoded.Data.Result {
		for _, raw := range decoded.Data.Result[i].Values {
			if len(raw) < 2 {
				continue
			}
			at, err := number(raw[0])
			if err != nil {
				continue
			}
			value, err := number(raw[1])
			if err != nil {
				continue
			}
			decoded.Data.Result[i].Points = append(decoded.Data.Result[i].Points, Point{At: int64(at * 1000), Value: value})
		}
	}
	return decoded.Data.Result, nil
}

func number(raw any) (float64, error) {
	switch value := raw.(type) {
	case string:
		return strconv.ParseFloat(value, 64)
	case float64:
		return value, nil
	default:
		return 0, fmt.Errorf("invalid metric value")
	}
}
