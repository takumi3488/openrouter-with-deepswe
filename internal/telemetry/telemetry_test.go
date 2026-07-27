package telemetry

import "testing"

func TestStripScheme(t *testing.T) {
	cases := map[string]string{
		"http://otel-gateway.monitoring.svc.cluster.local:4317":  "otel-gateway.monitoring.svc.cluster.local:4317",
		"https://otel-gateway.monitoring.svc.cluster.local:4317": "otel-gateway.monitoring.svc.cluster.local:4317",
		"localhost:4317": "localhost:4317",
	}
	for in, want := range cases {
		if got := stripScheme(in); got != want {
			t.Errorf("stripScheme(%q) = %q, want %q", in, got, want)
		}
	}
}
