// uagent docker-controller: the only service allowed to talk to Docker.
//
// It exposes a deliberately tiny API: create a disposable copy of the
// backend container, and destroy one by id. Callers cannot choose an image,
// a command, mounts or any other Docker parameter. The only per-container
// input is the content of the config.json the copy starts with. It speaks
// the Docker Engine API over the local unix socket, so no Docker CLI is needed.
//
// The shared secret with the backend needs no setup: when CONTROLLER_TOKEN is
// not set, a token is generated once and stored in CONTROLLER_TOKEN_FILE,
// which both services mount (a small named volume in docker-compose.yml).
package main

import (
	"archive/tar"
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
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
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	labelManaged     = "uagent.disposable"
	labelOwner       = "uagent.owner"
	labelComposeSvc  = "com.docker.compose.service"
	namePrefix       = "uagent-disp-"
	pidsLimit        = 1024
	socketPath       = "/var/run/docker.sock"
	tokenFileDefault = "/run/uagent-controller/token"
	// maxConfigBytes bounds the config.json a caller may seed into a copy.
	maxConfigBytes = 1 << 20
	configFileName = "config.json"
	dataMountPoint = "/data"
)

var (
	errBadRequest = errors.New("bad request")
	errNotFound   = errors.New("not found")

	errUnauth     = errors.New("unauthorized")
	containerIDRe = regexp.MustCompile(`^[a-f0-9]{12,64}$`)
	token         = os.Getenv("CONTROLLER_TOKEN")
	tokenFile     = envOr("CONTROLLER_TOKEN_FILE", tokenFileDefault)
	sourceService = envOr("SOURCE_SERVICE", "backend")
	listenAddr    = envOr("LISTEN_ADDR", ":8090")
)

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// ---------- Docker Engine API (subsets) ----------

type containerConfig struct {
	Env        []string          `json:"Env"`
	Labels     map[string]string `json:"Labels"`
	Cmd        []string          `json:"Cmd"`
	Entrypoint []string          `json:"Entrypoint"`
	WorkingDir string            `json:"WorkingDir"`
	User       string            `json:"User"`
	Image      string            `json:"Image"`
}

type containerInspect struct {
	ID     string          `json:"Id"`
	Name   string          `json:"Name"`
	Image  string          `json:"Image"`
	Config containerConfig `json:"Config"`
	State  struct {
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
	return c.doReq(req, out)
}

// doReq performs a request against the Docker API and decodes a JSON answer.
func (c *dockerClient) doReq(req *http.Request, out any) error {
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
		return fmt.Errorf("docker %s %s: %s", req.Method, req.URL.Path, strings.TrimSpace(string(data)))
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

// imageExists reports whether the daemon still knows an image reference.
func (c *dockerClient) imageExists(ref string) bool {
	if strings.TrimSpace(ref) == "" {
		return false
	}
	return c.do("GET", "/images/"+ref+"/json", nil, nil) == nil
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

// chooseImage picks the image a disposable copy is created from.
//
// A running container's inspect data keeps pointing at the image ID it was
// created from. A rebuild ("docker compose up --build") replaces that image,
// and the old ID can disappear from the image store while the container keeps
// running. Creating a copy then fails with "No such image". Prefer the exact
// image the template runs on, and fall back to the reference (tag) it was
// created from, which now points at the rebuilt image.
func chooseImage(src *containerInspect, imageExists func(string) bool) (string, error) {
	for _, ref := range []string{src.Image, src.Config.Image} {
		if imageExists(ref) {
			return ref, nil
		}
	}
	return "", fmt.Errorf("image %q is not available in Docker; rebuild the %q service",
		src.Config.Image, sourceService)
}

// create starts a new container from the template's image and environment,
// attached to the compose network, with a private /data volume. seedConfig,
// when not empty, is written into that volume, at the path where the template
// itself keeps its config file, before the container starts.
func (c *dockerClient) create(src *containerInspect, net *networkInspect, name, owner, image string, seedConfig []byte) (*containerInspect, error) {
	body := map[string]any{
		"Image":      image,
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
	if len(seedConfig) > 0 {
		relPath := seedRelativePath(src.Config.Env)
		archive, err := configTar(seedConfig, relPath)
		if err != nil {
			_ = c.remove(created.ID)
			_ = c.removeVolume(name)
			return nil, err
		}
		// Extracting into the not-yet-started container writes through the
		// volume mount, so the app finds the file on its first start.
		if err := c.putArchive(created.ID, dataMountPoint, archive); err != nil {
			_ = c.remove(created.ID)
			_ = c.removeVolume(name)
			return nil, fmt.Errorf("write %s: %w", relPath, err)
		}
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

// putArchive extracts a tar stream into a container's filesystem at path.
func (c *dockerClient) putArchive(id, path string, archive []byte) error {
	req, err := http.NewRequest(http.MethodPut, "http://docker/containers/"+id+"/archive?path="+url.QueryEscape(path), bytes.NewReader(archive))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-tar")
	return c.doReq(req, nil)
}

// seedRelativePath returns where, relative to the private /data volume, the
// template keeps its config file: its own CONFIG_PATH when that lies inside
// /data, otherwise "config.json". Only the location is derived here; the
// content is whatever the backend sends.
func seedRelativePath(env []string) string {
	for _, e := range env {
		if value, ok := strings.CutPrefix(e, "CONFIG_PATH="); ok && value != "" {
			if rel, ok := strings.CutPrefix(filepath.Clean(value), dataMountPoint+"/"); ok {
				return rel
			}
		}
	}
	return configFileName
}

// configTar wraps a config file in a tar stream for putArchive, including
// entries for any parent folders so they are created on extraction.
func configTar(configJSON []byte, relPath string) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	// Parent folders first, by their full relative path, so extraction
	// creates them in the fresh volume.
	prefix := ""
	for _, part := range strings.Split(path.Dir(relPath), "/") {
		if part == "." || part == "" {
			continue
		}
		prefix = path.Join(prefix, part)
		if err := tw.WriteHeader(&tar.Header{Name: prefix + "/", Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
			return nil, err
		}
	}
	hdr := &tar.Header{
		Name:    relPath,
		Mode:    0o644,
		Size:    int64(len(configJSON)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, err
	}
	if _, err := tw.Write(configJSON); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decodeSeedConfig decodes the base64 config.json a caller wants a copy to
// start with. An empty value means "no config to seed".
func decodeSeedConfig(b64 string) ([]byte, error) {
	if strings.TrimSpace(b64) == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("%w: config is not valid base64", errBadRequest)
	}
	if len(raw) == 0 || len(raw) > maxConfigBytes {
		return nil, fmt.Errorf("%w: config size out of range", errBadRequest)
	}
	if !json.Valid(raw) || !strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		return nil, fmt.Errorf("%w: config is not a JSON object", errBadRequest)
	}
	return raw, nil
}

// loadOrCreateToken returns the shared token stored in path, creating it on
// first use. It returns "" when no token can be established.
func loadOrCreateToken(path string) string {
	if path == "" {
		return ""
	}
	if data, err := os.ReadFile(path); err == nil {
		if t := strings.TrimSpace(string(data)); len(t) >= 32 {
			return t
		}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		log.Printf("cannot generate token: %v", err)
		return ""
	}
	t := hex.EncodeToString(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("cannot create token directory %s: %v", filepath.Dir(path), err)
		return ""
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(t+"\n"), 0o644); err != nil {
		log.Printf("cannot write token file %s: %v", path, err)
		return ""
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("cannot publish token file %s: %v", path, err)
		return ""
	}
	log.Printf("generated a new shared token in %s", path)
	return t
}

// ---------- HTTP API ----------

type apiHandler func(http.ResponseWriter, *http.Request) error

func main() {
	if token == "" {
		token = loadOrCreateToken(tokenFile)
	}
	// Without a token the service stays up but refuses everything, so the
	// rest of the stack is unaffected and the feature is simply unavailable.
	if token == "" {
		log.Print("no shared token available (set CONTROLLER_TOKEN or mount a writable volume at " + tokenFile + "): disposable containers are disabled")
	}
	d := newDockerClient()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", withAuth(handleHealth(d)))
	mux.HandleFunc("POST /containers", withAuth(handleCreate(d)))
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

func handleCreate(d *dockerClient) apiHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		var req struct {
			Owner  string `json:"owner"`
			Config string `json:"config"`
		}
		// base64 of the config plus overhead
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*maxConfigBytes)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("%w: invalid JSON", errBadRequest)
		}
		owner := strings.TrimSpace(req.Owner)
		if len(owner) > 128 || strings.ContainsAny(owner, "\n\r") {
			return fmt.Errorf("%w: invalid owner", errBadRequest)
		}

		seed, err := decodeSeedConfig(req.Config)
		if err != nil {
			return err
		}

		net, err := d.ownNetwork()
		if err != nil {
			return err
		}
		src, err := d.source(net)
		if err != nil {
			return err
		}
		image, err := chooseImage(src, d.imageExists)
		if err != nil {
			return err
		}

		// Copy-on-write: same image (shared read-only layers) plus a thin
		// per-container write layer. Nothing is copied from disk.
		name := namePrefix + randHex(4)
		ci, err := d.create(src, net, name, owner, image, seed)
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
