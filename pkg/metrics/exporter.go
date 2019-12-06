package metrics

type MetricsExporter interface {
	Export() error
}
