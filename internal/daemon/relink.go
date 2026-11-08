package daemon

import (
	"log/slog"
	"slices"
	"strings"

	"github.com/dps/berth/internal/state"
)

// maxPastGateways bounds the gateways remembered: macOS alternates between
// a couple of subnets, so a handful covers any machine's history.
const maxPastGateways = 8

// rememberGateway makes gw the gateway, keeping the one it replaces so
// links that still name it can be found and rewritten.
func rememberGateway(cfg *state.Config, gw string) {
	if cfg.Gateway != "" && cfg.Gateway != gw && !slices.Contains(cfg.PastGateways, cfg.Gateway) {
		cfg.PastGateways = append(cfg.PastGateways, cfg.Gateway)
	}
	cfg.PastGateways = slices.DeleteFunc(cfg.PastGateways, func(g string) bool { return g == gw || g == "" })
	if n := len(cfg.PastGateways); n > maxPastGateways {
		cfg.PastGateways = cfg.PastGateways[n-maxPastGateways:]
	}
	cfg.Gateway = gw
}

// regateway points a value that names a past gateway at gw: service URLs
// (user:pass@gw:port) and the AI and tools gateways' (http://gw:port).
func regateway(v string, past []string, gw string) (string, bool) {
	changed := false
	for _, old := range past {
		if old == "" || old == gw {
			continue
		}
		for _, sep := range []string{"@", "//"} {
			if strings.Contains(v, sep+old+":") {
				v = strings.ReplaceAll(v, sep+old+":", sep+gw+":")
				changed = true
			}
		}
	}
	return v, changed
}

// relinkGateways rewrites past gateways to the current one in every app's
// env and in every release's saved env, which is what containers (web,
// workers, cron, restarts, rollbacks) are started from. It is idempotent and
// runs on every reconcile, before anything starts. It returns the apps whose
// current release was rewritten and has a container: if that container is
// running, it was started with the old gateway and needs a restart.
func (d *Daemon) relinkGateways(cfg *state.Config) (stale []string) {
	if len(cfg.PastGateways) == 0 || cfg.Gateway == "" {
		return nil
	}
	apps, _ := d.Store.Apps()
	for _, a := range apps {
		if env, err := d.Store.Env(a.Name); err == nil {
			changed := false
			for k, v := range env {
				if nv, ok := regateway(v, cfg.PastGateways, cfg.Gateway); ok {
					env[k] = nv
					changed = true
				}
			}
			if changed {
				if err := d.Store.SaveEnv(a.Name, env); err != nil {
					slog.Error("relink env", "app", a.Name, "err", err)
				} else {
					slog.Info("pointed env at the new network gateway", "app", a.Name, "gateway", cfg.Gateway)
				}
			}
		}
		cur := d.Store.Current(a.Name)
		rels, _ := d.Store.Releases(a.Name)
		for _, r := range rels {
			changed := false
			for k, v := range r.Env {
				if nv, ok := regateway(v, cfg.PastGateways, cfg.Gateway); ok {
					r.Env[k] = nv
					changed = true
				}
			}
			if !changed {
				continue
			}
			if err := d.Store.SaveRelease(a.Name, r); err != nil {
				slog.Error("relink release", "app", a.Name, "release", r.N, "err", err)
				continue
			}
			if r.N == cur && r.Container != "" {
				stale = append(stale, a.Name)
			}
		}
	}
	return stale
}
