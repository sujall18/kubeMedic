package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	IncidentsDetected = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubemedic_incidents_detected_total",
			Help: "Total number of Kubernetes incidents detected by KubeMedic.",
		},
		[]string{"reason"},
	)

	RemediationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubemedic_remediations_total",
			Help: "Total number of remediation attempts performed by KubeMedic.",
		},
		[]string{"action", "result"},
	)

	RemediationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "kubemedic_remediation_duration_seconds",
			Help: "Time spent performing KubeMedic remediation actions.",
		},
		[]string{"action"},
	)

	ActiveIncidents = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "kubemedic_active_incidents",
			Help: "Number of incidents currently being processed by KubeMedic.",
		},
	)

	IncidentsResolved = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubemedic_incidents_resolved_total",
			Help: "Total number of incidents successfully resolved by KubeMedic.",
		},
		[]string{"reason"},
	)

	IncidentsExhausted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubemedic_incidents_exhausted_total",
			Help: "Total number of incidents that exhausted KubeMedic remediation attempts.",
		},
		[]string{"reason"},
	)

	VerificationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubemedic_verification_total",
			Help: "Total number of remediation verification attempts.",
		},
		[]string{"result"},
	)

	RecoveryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "kubemedic_recovery_duration_seconds",
			Help: "Time from incident detection until successful recovery.",
		},
		[]string{"reason"},
	)
)

func init() {
	prometheus.MustRegister(
		IncidentsDetected,
		RemediationsTotal,
		RemediationDuration,
		ActiveIncidents,
		IncidentsResolved,
		IncidentsExhausted,
		VerificationTotal,
		RecoveryDuration,
	)
}

func Handler() http.Handler {
	return promhttp.Handler()
}
