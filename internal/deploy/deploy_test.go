package deploy

import "testing"

func TestAllHealthy(t *testing.T) {
	cases := []struct {
		name string
		svcs []Svc
		want bool
	}{
		{"empty", nil, false},
		{"running-no-healthcheck", []Svc{{Name: "a", Running: true, Health: "running"}}, true},
		{"healthy", []Svc{{Name: "a", Running: true, HasHealth: true, Health: "healthy"}}, true},
		{"starting", []Svc{{Name: "a", Running: true, HasHealth: true, Health: "starting"}}, false},
		{"unhealthy", []Svc{{Name: "a", Running: true, HasHealth: true, Health: "unhealthy"}}, false},
		{"not-running", []Svc{{Name: "a", Running: false, Health: "exited"}}, false},
		{"mixed", []Svc{
			{Name: "a", Running: true, HasHealth: true, Health: "healthy"},
			{Name: "b", Running: true, Health: "running"},
		}, true},
		{"mixed-sick", []Svc{
			{Name: "a", Running: true, HasHealth: true, Health: "healthy"},
			{Name: "b", Running: true, HasHealth: true, Health: "unhealthy"},
		}, false},
	}
	for _, c := range cases {
		if got := allHealthy(c.svcs); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestUnhealthyNames(t *testing.T) {
	got := unhealthyNames([]Svc{
		{Name: "b", Running: true, HasHealth: true, Health: "unhealthy"},
		{Name: "a", Running: false, Health: "exited"},
		{Name: "c", Running: true, HasHealth: true, Health: "healthy"},
	})
	if got != "a(exited), b(unhealthy)" {
		t.Errorf("got %q", got)
	}
}
