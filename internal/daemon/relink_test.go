package daemon

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"github.com/dps/berth/internal/runtime"
	"github.com/dps/berth/internal/state"
)

func TestRegateway(t *testing.T) {
	past := []string{"192.168.65.1", "192.168.66.1"}
	for _, c := range []struct{ in, want string }{
		{"postgres://u:p@192.168.65.1:6001/db", "postgres://u:p@192.168.64.1:6001/db"},
		{"http://192.168.66.1:7790/v1", "http://192.168.64.1:7790/v1"},
		{"redis://default:x@192.168.65.1:6002", "redis://default:x@192.168.64.1:6002"},
		// Not a gateway address: a different host, or no port after it.
		{"http://192.168.65.10:7790/v1", "http://192.168.65.10:7790/v1"},
		{"see 192.168.65.1 for details", "see 192.168.65.1 for details"},
		{"postgres://u:p@192.168.64.1:6001/db", "postgres://u:p@192.168.64.1:6001/db"},
	} {
		got, changed := regateway(c.in, past, "192.168.64.1")
		if got != c.want || changed != (c.in != c.want) {
			t.Errorf("regateway(%q) = %q, %v; want %q", c.in, got, changed, c.want)
		}
	}
}

func TestRememberGateway(t *testing.T) {
	cfg := &state.Config{Gateway: "192.168.64.1"}
	rememberGateway(cfg, "192.168.65.1")
	rememberGateway(cfg, "192.168.64.1") // and back again
	if cfg.Gateway != "192.168.64.1" || !slices.Equal(cfg.PastGateways, []string{"192.168.65.1"}) {
		t.Fatalf("gateway %s, past %v", cfg.Gateway, cfg.PastGateways)
	}
	fresh := &state.Config{}
	rememberGateway(fresh, "192.168.64.1")
	if len(fresh.PastGateways) != 0 {
		t.Fatalf("a first gateway has no past: %v", fresh.PastGateways)
	}
}

// After a reboot hands the network a new subnet, apps' env and their
// releases' saved env point at the new gateway before anything starts, and
// a container still running with the old one is replaced. If berthd stops
// before the rewrite finishes, the next reconcile finishes it.
func TestGatewayChangeRelinksAppsAndReleases(t *testing.T) {
	d, rt, repo := newTestDaemon(t)
	params, _ := json.Marshal(CreateParams{Name: "web", Repo: repo})
	if _, err := d.hCreate(d.ctx, params, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	d.waitIdle(t, "web")
	const old, gw = "10.9.9.1", "127.0.0.1" // the fake runtime's gateway is 127.0.0.1
	stale := map[string]string{
		"DATABASE_URL":       "postgres://u:p@" + old + ":6001/db",
		"ANTHROPIC_BASE_URL": "http://" + old + ":7790/anthropic",
		"ELSEWHERE":          "https://example.com:443/" + old,
	}
	makeStale := func() {
		env, _ := d.Store.Env("web")
		for k, v := range stale {
			env[k] = v
		}
		d.Store.SaveEnv("web", env)
		rel := d.Store.CurrentRelease("web")
		for k, v := range stale {
			rel.Env[k] = v
		}
		d.Store.SaveRelease("web", rel)
	}
	check := func(when string) {
		t.Helper()
		env, _ := d.Store.Env("web")
		rel := d.Store.CurrentRelease("web")
		for _, e := range []map[string]string{env, rel.Env} {
			if e["DATABASE_URL"] != "postgres://u:p@"+gw+":6001/db" || e["ANTHROPIC_BASE_URL"] != "http://"+gw+":7790/anthropic" {
				t.Fatalf("%s: not relinked: %v", when, e)
			}
			if e["ELSEWHERE"] != stale["ELSEWHERE"] {
				t.Fatalf("%s: rewrote a value that isn't a gateway URL: %q", when, e["ELSEWHERE"])
			}
		}
	}

	makeStale()
	cfg, _ := d.Store.Config()
	cfg.Gateway = old
	d.Store.SaveConfig(cfg)
	before := d.Store.CurrentRelease("web").Container
	d.reconcile(d.ctx)

	cfg, _ = d.Store.Config()
	if cfg.Gateway != gw || !slices.Equal(cfg.PastGateways, []string{old}) {
		t.Fatalf("gateway %s, past %v", cfg.Gateway, cfg.PastGateways)
	}
	check("after the change")
	rel := d.Store.CurrentRelease("web")
	cs, _ := rt.List(d.ctx)
	if c, ok := runtime.Find(cs, rel.Container); rel.Container == before || !ok || !c.Running() {
		t.Fatalf("container started with the old gateway wasn't replaced: was %s, now %s", before, rel.Container)
	}

	// berthd saved the new gateway but stopped before rewriting: the gateway
	// hasn't changed since, and the next reconcile still fixes it.
	makeStale()
	d.reconcile(d.ctx)
	check("on the next reconcile")
}
