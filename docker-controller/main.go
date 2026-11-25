// uagent docker-controller: the only service allowed to talk to Docker.
//
// It exposes a deliberately tiny API: create a disposable copy of the
// backend container, and destroy one by id. Callers cannot choose an image,
// a command, mounts or any other Docker parameter. It speaks the Docker
// Engine API over the local unix socket, so no Docker CLI is needed.
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	labelManaged    = "uagent.disposable"
	labelOwner      = "uagent.owner"
	labelComposeSvc = "com.docker.compose.service"
	namePrefix      = "uagent-disp-"
	defaultMax      = 8
	pidsLimit       = 1024
	socketPath      = "/var/run/docker.sock"
)

var (
	errBadRequest = errors.New("bad request")
	errNotFound   = errors.New("not found")
	errConflict   = errors.New("conflict")

	errUnauth     = errors.New("unauthorized")
	containerIDRe = regexp.MustCompile(`^[a-f0-9]{12,64}$`)
	token         = os.Getenv("CONTROLLER_TOKEN")
	sourceService = envOr("SOURCE_SERVICE", "backend")
	listenAddr    = envOr("LISTEN_ADDR", ":8090")
)

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func maxContainers() int {
	n, err := strconv.Atoi(envOr("MAX_CONTAINERS", strconv.Itoa(defaultMax)))
	if err != nil || n < 1 {
		return defaultMax
	}
	return n
}

// ---------- Docker Engine API (subsets) ----------

type containerInspect struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Image  string `json:"Image"`
	Config struct {
		Env        []string          `json:"Env"`
		Labels     map[string]string `json:"Labels"`
		Cmd        []string          `json:"Cmd"`
		Entrypoint []string          `json:"Entrypoint"`
		WorkingDir string            `json:"WorkingDir"`
		User       string            `json:"User"`
	} `json:"Config"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
	NetworkSettings struct {
		Networks map[string]struct {
			IPAddress string `json:"IPAddress"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

type networkInspect struct {
	ID         string `json:"Id"`
	Name       string `json:"Name"`
	Containers map[string]struct {
		Name        string `json:"Name"`
		IPv4Address string `json:"IPv4Address"`
	} `json:"Containers"`
}

// ---------- docker client ----------

type dockerClient struct {
	http *http.Client
}

func newDockerClient() *dockerClient {
	tr := &http.Transport{}
	tr.Dial = func(_, _ string) (net.Conn, error) { return net.Dial("unix", socketPath) }
	return &dockerClient{http: &http.Client{Transport: tr, Timeout: 60 * time.Second}}
}

func (c *dockerClient) do(method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "http://docker"+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("docker: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("docker %s %s: %s", method, path, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

func (c *dockerClient) inspectContainer(id string) (*containerInspect, error) {
	var ci containerInspect
	if err := c.do("GET", "/containers/"+id+"/json", nil, &ci); err != nil {
		return nil, err
	}
	return &ci, nil
}

// ownNetwork finds the network this sidecar is attached to, which is the
// compose network the disposable containers must join as well.
func (c *dockerClient) ownNetwork() (*networkInspect, error) {
	self := map[string]bool{}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			self[ipn.IP.String()] = true
		}
	}
	var list []struct {
		ID string `json:"Id"`
	}
	if err := c.do("GET", "/networks", nil, &list); err != nil {
		return nil, err
	}
	for _, n := range list {
		var full networkInspect
		// A network may vanish between list and inspect; skip such races.
		if err := c.do("GET", "/networks/"+n.ID, nil, &full); err != nil {
			continue
		}
		for _, ep := range full.Containers {
			if self[strings.SplitN(ep.IPv4Address, "/", 2)[0]] {
				return &full, nil
			}
		}
	}
	return nil, errors.New("cannot determine the docker network of the controller")
}

// source returns the running compose service container that serves as the
// template, plus the network it lives on.
func (c *dockerClient) source(net *networkInspect) (*containerInspect, error) {
	for id := range net.Containers {
		ci, err := c.inspectContainer(id)
		if err != nil {
			continue
		}
		if ci.Config.Labels[labelComposeSvc] == sourceService &&
			ci.Config.Labels[labelManaged] != "true" && ci.State.Running {
			return ci, nil
		}
	}
	return nil, fmt.Errorf("no running compose service %q on the compose network", sourceService)
}

func (c *dockerClient) managedCount() (int, error) {
	var list []json.RawMessage
	filters := `{"label":["` + labelManaged + `=true"]}`
	path := "/containers/json?all=1&filters=" + url.QueryEscape(filters)
	if err := c.do("GET", path, nil, &list); err != nil {
		return 0, err
	}
	return len(list), nil
}

func (c *dockerClient) remove(id string) error {
	return c.do("DELETE", "/containers/"+id+"?force=1&v=1", nil, nil)
}

func (c *dockerClient) removeVolume(name string) error {
	return c.do("DELETE", "/volumes/"+name, nil, nil)
}

// create starts a new container from the template's image and environment,
// attached to the compose network, with a private /data volume.
func (c *dockerClient) create(src *containerInspect, net *networkInspect, name, owner string) (*containerInspect, error) {
	body := map[string]any{
		"Image":      src.Image,
		"Labels":     map[string]string{labelManaged: "true", labelOwner: owner},
		"Env":        templateEnv(src.Config.Env),
		"Cmd":        src.Config.Cmd,
		"Entrypoint": src.Config.Entrypoint,
		"WorkingDir": src.Config.WorkingDir,
		"User":       src.Config.User,
		"HostConfig": map[string]any{
			"PidsLimit": pidsLimit,
			"Binds":     []string{name + ":/data"},
		},
		"NetworkingConfig": map[string]any{
			"EndpointsConfig": map[string]any{net.Name: map[string]any{}},
		},
	}
	var created struct {
		ID string `json:"Id"`
	}
	if err := c.do("POST", "/containers/create?name="+name, body, &created); err != nil {
		return nil, err
	}
	startErr := c.do("POST", "/containers/"+created.ID+"/start", nil, nil)
	if startErr != nil {
		_ = c.remove(created.ID)
		_ = c.removeVolume(name)
		return nil, startErr
	}
	// The IP is assigned asynchronously right after start; wait briefly for it.
	var ci *containerInspect
	var err error
	for i := 0; i < 50; i++ {
		ci, err = c.inspectContainer(created.ID)
		if err == nil && ci.NetworkSettings.Networks[net.Name].IPAddress != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		_ = c.remove(created.ID)
		return nil, err
	}
	if ci.NetworkSettings.Networks[net.Name].IPAddress == "" {
		_ = c.remove(created.ID)
		_ = c.removeVolume(name)
		return nil, errors.New("disposable container did not get an IP address")
	}
	return ci, nil
}

// templateEnv returns the template's environment minus the credentials for
// this controller, so a disposable container cannot create more containers.
func templateEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "CONTROLLER_") {
			continue
		}
		out = append(out, e)
	}
	return out
}

// ---------- HTTP API ----------

type apiHandler func(http.ResponseWriter, *http.Request) error

func main() {
	// Without a token the service stays up but refuses everything, so the
	// rest of the stack is unaffected and the feature is simply unavailable.
	if token == "" {
		log.Print("CONTROLLER_TOKEN is not set: disposable containers are disabled")
	}
	d := newDockerClient()
	max := maxContainers()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", withAuth(handleHealth(d)))
	mux.HandleFunc("POST /containers", withAuth(handleCreate(d, max)))
	mux.HandleFunc("DELETE /containers/{id}", withAuth(handleDelete(d)))

	log.Printf("docker-controller listening on %s", listenAddr)
	srv := &http.Server{Addr: listenAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func withAuth(h apiHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			writeErr(w, http.StatusServiceUnavailable, errors.New("docker-controller is not configured: CONTROLLER_TOKEN is empty"))
			return
		}
		presented := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(presented, []byte("Bearer "+token)) != 1 {
			writeErr(w, http.StatusUnauthorized, errUnauth)
			return
		}
		if err := h(w, r); err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, errBadRequest):
				status = http.StatusBadRequest
			case errors.Is(err, errNotFound):
				status = http.StatusNotFound
			case errors.Is(err, errConflict):
				status = http.StatusConflict
			}
			log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
			writeErr(w, status, err)
		}
	}
}

func writeErr(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func writeOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano()&0xffffffff, 16)
	}
	return hex.EncodeToString(b)
}

// handleHealth also proves that Docker is actually reachable, so callers can
// treat it as "this deployment can create disposable containers".
func handleHealth(d *dockerClient) apiHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		if _, err := d.managedCount(); err != nil {
			return fmt.Errorf("docker is not reachable: %w", err)
		}
		writeOK(w, map[string]string{"status": "ok"})
		return nil
	}
}

func handleCreate(d *dockerClient, max int) apiHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		var req struct {
			Owner string `json:"owner"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("%w: invalid JSON", errBadRequest)
		}
		owner := strings.TrimSpace(req.Owner)
		if len(owner) > 128 || strings.ContainsAny(owner, "\n\r") {
			return fmt.Errorf("%w: invalid owner", errBadRequest)
		}

		n, err := d.managedCount()
		if err != nil {
			return err
		}
		if n >= max {
			return fmt.Errorf("%w: limit of %d disposable containers reached", errConflict, max)
		}

		net, err := d.ownNetwork()
		if err != nil {
			return err
		}
		src, err := d.source(net)
		if err != nil {
			return err
		}

		// Copy-on-write: same image (shared read-only layers) plus a thin
		// per-container write layer. Nothing is copied from disk.
		name := namePrefix + randHex(4)
		ci, err := d.create(src, net, name, owner)
		if err != nil {
			return err
		}

		log.Printf("created %s (%s)", name, ci.ID[:12])
		writeOK(w, map[string]any{
			"id":        ci.ID,
			"name":      name,
			"ip":        ci.NetworkSettings.Networks[net.Name].IPAddress,
			"createdAt": time.Now().UnixMilli(),
		})
		return nil
	}
}

func handleDelete(d *dockerClient) apiHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		id := r.PathValue("id")
		if !containerIDRe.MatchString(id) {
			return fmt.Errorf("%w: invalid container id", errBadRequest)
		}
		ci, err := d.inspectContainer(id)
		if err != nil {
			return fmt.Errorf("%w: %v", errNotFound, err)
		}
		if ci.Config.Labels[labelManaged] != "true" {
			return fmt.Errorf("%w: not a disposable container", errNotFound)
		}
		if err := d.remove(id); err != nil {
			return err
		}
		// The private data volume carries the container's name.
		_ = d.removeVolume(strings.TrimPrefix(ci.Name, "/"))
		log.Printf("destroyed %s", strings.TrimPrefix(ci.Name, "/"))
		writeOK(w, map[string]string{"result": "destroyed"})
		return nil
	}
}
