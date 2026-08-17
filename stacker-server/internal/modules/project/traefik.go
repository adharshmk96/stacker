package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

// Traefik routing for deployed projects.
//
// The installed Traefik runs the file provider against a watched directory (see
// deploy/traefik/traefik.yml), so publishing a hostname means writing a file
// there — no container labels, and no Traefik restart. Each environment owns
// exactly one file, named after its id, so a redeploy replaces its own routes
// and can never disturb another environment's.
//
// Traefik reaches a service by its swarm DNS name on the shared proxy overlay,
// which is why buildOverride attaches every routed service to that network.

// certResolver is the ACME resolver install.sh configures. `auto` domains ask
// for a certificate from it.
const certResolver = "letsencrypt"

// routes is the dynamic-config document written per environment.
type routes struct {
	HTTP routesHTTP `yaml:"http"`
}

type routesHTTP struct {
	Routers     map[string]routesRouter     `yaml:"routers,omitempty"`
	Services    map[string]routesService    `yaml:"services,omitempty"`
	Middlewares map[string]routesMiddleware `yaml:"middlewares,omitempty"`
}

type routesRouter struct {
	Rule        string     `yaml:"rule"`
	EntryPoints []string   `yaml:"entryPoints"`
	Service     string     `yaml:"service"`
	Middlewares []string   `yaml:"middlewares,omitempty"`
	TLS         *routesTLS `yaml:"tls,omitempty"`
}

// routesTLS with no resolver is a valid Traefik block: it means "serve TLS with
// whatever certificate is already loaded", which is what `custom` asks for.
type routesTLS struct {
	CertResolver string `yaml:"certResolver,omitempty"`
}

type routesService struct {
	LoadBalancer routesLoadBalancer `yaml:"loadBalancer"`
}

type routesLoadBalancer struct {
	Servers []routesServer `yaml:"servers"`
}

type routesServer struct {
	URL string `yaml:"url"`
}

type routesMiddleware struct {
	RedirectRegex *routesRedirect `yaml:"redirectRegex,omitempty"`
}

type routesRedirect struct {
	Regex       string `yaml:"regex"`
	Replacement string `yaml:"replacement"`
	Permanent   bool   `yaml:"permanent"`
}

// router is the seam through which routes are written. It exists so the deploy
// engine can be tested against a temporary directory, and so a build of stacker
// running without the Traefik volume mounted fails with ErrTraefikMissing rather
// than scattering files into a path that means nothing.
type router struct {
	// dir is the watched dynamic-config directory.
	dir string
}

func newRouter(dynamicPath string) *router {
	return &router{dir: filepath.Dir(dynamicPath)}
}

// routeFile is where one environment's routes live. The `stacker-project-`
// prefix keeps stacker's own file (stacker.yml) and any file the operator wrote
// by hand clearly separate from generated ones.
func (r *router) routeFile(envID string) string {
	return filepath.Join(r.dir, "stacker-project-"+envID+".yml")
}

// Apply writes an environment's routes, replacing whatever was there. Passing no
// domains removes the file, so an environment that loses its last hostname stops
// being routed instead of keeping a route to a service that no longer exists.
func (r *router) Apply(envID, stack string, domains []Domain) error {
	if len(domains) == 0 {
		return r.Remove(envID)
	}
	if _, err := os.Stat(r.dir); err != nil {
		return ErrTraefikMissing
	}

	content, err := renderRoutes(stack, domains)
	if err != nil {
		return err
	}

	// Written through a temp file and renamed: Traefik watches the directory and
	// would otherwise reload a half-written document.
	target := r.routeFile(envID)
	tmp, err := os.CreateTemp(r.dir, ".stacker-project-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck // renamed on success

	if err = tmp.Chmod(0o644); err == nil {
		_, err = tmp.WriteString(content)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

// Remove deletes an environment's routes. A file that is already gone is not an
// error: removing routes is called on delete and on rollback, and both have to
// be safe to run twice.
func (r *router) Remove(envID string) error {
	err := os.Remove(r.routeFile(envID))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// renderRoutes builds the dynamic config for one environment's domains.
func renderRoutes(stack string, domains []Domain) (string, error) {
	doc := routes{HTTP: routesHTTP{
		Routers:     map[string]routesRouter{},
		Services:    map[string]routesService{},
		Middlewares: map[string]routesMiddleware{},
	}}

	for i, domain := range domains {
		// The key is positional rather than derived from the host: a hostname
		// contains dots and can contain a `*`, neither of which reads well as a
		// Traefik key, and the position is already unique within the file.
		key := fmt.Sprintf("%s-%d", stack, i)

		rule := "Host(`" + domain.Host + "`)"
		var middlewares []string
		if domain.RedirectWww {
			// Both hosts have to match for the redirect to ever be reached —
			// Traefik cannot redirect a request that no router accepted.
			rule = "Host(`" + domain.Host + "`, `www." + domain.Host + "`)"
			middlewares = []string{key + "-www"}
			doc.HTTP.Middlewares[key+"-www"] = routesMiddleware{RedirectRegex: &routesRedirect{
				Regex:       `^(https?)://www\.(.+)$`,
				Replacement: "${1}://${2}",
				Permanent:   true,
			}}
		}

		router := routesRouter{
			Rule:        rule,
			EntryPoints: []string{"websecure"},
			Service:     key,
			Middlewares: middlewares,
		}
		switch domain.TLS {
		case TLSAuto:
			router.TLS = &routesTLS{CertResolver: certResolver}
		case TLSCustom:
			router.TLS = &routesTLS{}
		default:
			// Plain http. The installed static config redirects the `web`
			// entrypoint to `websecure`, so a `none` domain is only reachable
			// over http if that redirect has been turned off — which is the
			// operator's call, not something a project should rewrite.
			router.EntryPoints = []string{"web"}
		}
		doc.HTTP.Routers[key] = router

		doc.HTTP.Services[key] = routesService{LoadBalancer: routesLoadBalancer{
			Servers: []routesServer{{URL: fmt.Sprintf("http://%s_%s:%d", stack, domain.Service, domain.Port)}},
		}}
	}

	if len(doc.HTTP.Middlewares) == 0 {
		doc.HTTP.Middlewares = nil
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return "# Generated by stacker. Edits are overwritten on the next deploy.\n" + string(out), nil
}

// proxyServices is the set of compose services a routed environment needs on the
// proxy network — the targets of its domains, and nothing else.
func proxyServices(domains []Domain) map[string]bool {
	out := map[string]bool{}
	for _, domain := range domains {
		if name := strings.TrimSpace(domain.Service); name != "" {
			out[name] = true
		}
	}
	return out
}
