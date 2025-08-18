package metrics

import (
	"strings"
	"sync/atomic"

	"github.com/go-gost/core/metrics"
)

const (
	// Number of services. Labels: host.
	MetricServicesGauge metrics.MetricName = "gost_services"
	// Total service requests. Labels: host, service, client.
	MetricServiceRequestsCounter metrics.MetricName = "gost_service_requests_total"
	// Number of in-flight requests. Labels: host, service, client.
	MetricServiceRequestsInFlightGauge metrics.MetricName = "gost_service_requests_in_flight"
	// Request duration histogram. Labels: host, service.
	MetricServiceRequestsDurationObserver metrics.MetricName = "gost_service_request_duration_seconds"
	// Total service input data transfer size in bytes. Labels: host, service, client.
	MetricServiceTransferInputBytesCounter metrics.MetricName = "gost_service_transfer_input_bytes_total"
	// Total service output data transfer size in bytes. Labels: host, service, client.
	MetricServiceTransferOutputBytesCounter metrics.MetricName = "gost_service_transfer_output_bytes_total"
	// Chain node connect duration histogram. Labels: host, chain, node.
	MetricNodeConnectDurationObserver metrics.MetricName = "gost_chain_node_connect_duration_seconds"
	// Total service handler errors. Labels: host, service, client.
	MetricServiceHandlerErrorsCounter metrics.MetricName = "gost_service_handler_errors_total"
	// Total chain connect errors. Labels: host, chain, node.
	MetricChainErrorsCounter metrics.MetricName = "gost_chain_errors_total"
	// Total recorder records. Labels: host, recorder.
	MetricRecorderRecordsCounter metrics.MetricName = "gost_recorder_records_total"
	// Total GRPC plugin requests. Labels: host, service, method, status_code.
	MetricGRPCRequestsCounter metrics.MetricName = "gost_grpc_requests_total"
	// GRPC plugin request duration histogram. Labels: host, service, method.
	MetricGRPCRequestsDurationObserver metrics.MetricName = "gost_grpc_request_duration_seconds"
	// Time to first byte from proxy node histogram. Labels: host, node, connector_type.
	MetricProxyNodeTTFBObserver metrics.MetricName = "gost_proxy_node_ttfb_seconds"
	// Proxy node round trip time histogram. Labels: host, node, connector_type.
	MetricProxyNodeRoundTripObserver metrics.MetricName = "gost_proxy_node_roundtrip_seconds"
	// Transport layer time to first byte histogram. Labels: host, direction.
	MetricTransportTTFBObserver metrics.MetricName = "gost_transport_ttfb_seconds"
)

var (
	defaultMetrics metrics.Metrics = NewMetrics()
	enabled        atomic.Bool
)

func Enable(b bool) {
	enabled.Store(b)
}

func IsEnabled() bool {
	return enabled.Load()
}

func GetCounter(name metrics.MetricName, labels metrics.Labels) metrics.Counter {
	if service, ok := labels["service"]; ok {
		paths := strings.Split(service, "-")
		if len(paths) >= 3 {
			labels["service"] = strings.Join(paths[:3], "-")
		}
	}

	if IsEnabled() {
		return defaultMetrics.Counter(name, labels)
	}
	return noop.Counter(name, labels)
}

func GetGauge(name metrics.MetricName, labels metrics.Labels) metrics.Gauge {
	if service, ok := labels["service"]; ok {
		paths := strings.Split(service, "-")
		if len(paths) >= 3 {
			labels["service"] = strings.Join(paths[:3], "-")
		}
	}
	if IsEnabled() {
		return defaultMetrics.Gauge(name, labels)
	}
	return noop.Gauge(name, labels)
}

func GetObserver(name metrics.MetricName, labels metrics.Labels) metrics.Observer {
	if service, ok := labels["service"]; ok {
		paths := strings.Split(service, "-")
		if len(paths) >= 3 {
			labels["service"] = strings.Join(paths[:3], "-")
		}
	}
	if IsEnabled() {
		return defaultMetrics.Observer(name, labels)
	}
	return noop.Observer(name, labels)
}
