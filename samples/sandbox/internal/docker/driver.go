// Package docker implements core.Driver on the Docker Engine HTTP API, so each
// box is a real container.
package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sandbox/internal/core"
)

const (
	boxPrefix      = "sandbox-"
	labelSandbox   = "codeplat.sandbox"
	labelName      = "codeplat.sandbox.name"
	labelWorkspace = "codeplat.sandbox.workspace"
)

// Driver runs boxes as containers through the Engine API.
type Driver struct {
	eng *engine
}

var _ core.Driver = (*Driver)(nil)

// New connects to the Engine at host. host is "unix:///path/to.sock", or
// "tcp://h:p" / "http://h:p". A nil hc gets a new client that dials host with
// no timeout. Other schemes are an error.
func New(host string, hc *http.Client) (*Driver, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("docker: parse host %q: %w", host, err)
	}
	var network, address string
	switch u.Scheme {
	case "unix":
		if u.Path == "" {
			return nil, fmt.Errorf("docker: unix host %q has no socket path", host)
		}
		network, address = "unix", u.Path
	case "tcp", "http":
		if u.Host == "" {
			return nil, fmt.Errorf("docker: host %q has no address", host)
		}
		network, address = "tcp", u.Host
	default:
		return nil, fmt.Errorf("docker: unsupported host scheme %q in %q", u.Scheme, host)
	}
	if hc == nil {
		var dialer net.Dialer
		hc = &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, address)
			},
		}}
	}
	return &Driver{eng: &engine{hc: hc}}, nil
}

// containerPath is the Engine path of the box's container.
func containerPath(name string) string {
	return "/containers/" + url.PathEscape(boxPrefix+name)
}

func notFound(name string) error {
	return fmt.Errorf("%w: box %q", core.ErrNotFound, name)
}

// inspectResponse is the part of GET /containers/{name}/json the driver reads.
type inspectResponse struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"`
	State struct {
		Status string `json:"Status"`
	} `json:"State"`
	Created string `json:"Created"`
	Config  struct {
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
}

// Get returns the box, or core.ErrNotFound when it does not exist.
func (d *Driver) Get(ctx context.Context, name string) (core.Box, error) {
	var in inspectResponse
	err := d.eng.callJSON(ctx, http.MethodGet, containerPath(name)+"/json", nil, &in)
	if isStatus(err, http.StatusNotFound) {
		return core.Box{}, notFound(name)
	}
	if err != nil {
		return core.Box{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, in.Created)
	if err != nil {
		return core.Box{}, fmt.Errorf("docker: parse created time of %q: %w", name, err)
	}
	return core.Box{
		// Box.Name is the container name without its "sandbox-" prefix.
		Name:        strings.TrimPrefix(strings.TrimPrefix(in.Name, "/"), boxPrefix),
		Image:       in.Config.Image,
		Workspace:   in.Config.Labels[labelWorkspace],
		ContainerID: in.ID,
		State:       in.State.Status,
		CreatedAt:   created,
	}, nil
}

// Ensure creates the box if it is missing, starts it if it is not running,
// and returns it as it now is.
func (d *Driver) Ensure(ctx context.Context, spec core.BoxSpec) (core.Box, error) {
	box, err := d.Get(ctx, spec.Name)
	if errors.Is(err, core.ErrNotFound) {
		if err := d.create(ctx, spec); err != nil {
			return core.Box{}, err
		}
		box, err = d.Get(ctx, spec.Name)
	}
	if err != nil {
		return core.Box{}, err
	}
	if box.State != "running" {
		if err := d.eng.send(ctx, http.MethodPost, "/containers/"+box.ContainerID+"/start", nil); err != nil {
			return core.Box{}, err
		}
		box, err = d.Get(ctx, spec.Name)
	}
	return box, err
}

// createBody is the POST /containers/create request.
type createBody struct {
	Image      string            `json:"Image"`
	Cmd        []string          `json:"Cmd"`
	WorkingDir string            `json:"WorkingDir"`
	User       string            `json:"User"`
	Labels     map[string]string `json:"Labels"`
	HostConfig hostConfig        `json:"HostConfig"`
}

type hostConfig struct {
	Binds       []string `json:"Binds"`
	Memory      int64    `json:"Memory"`
	NanoCpus    int64    `json:"NanoCpus"`
	NetworkMode string   `json:"NetworkMode"`
}

// create makes the container. When the image is missing it pulls the image
// and tries once more.
func (d *Driver) create(ctx context.Context, spec core.BoxSpec) error {
	body := createBody{
		Image:      spec.Image,
		Cmd:        []string{"sleep", "infinity"},
		WorkingDir: spec.Workspace,
		User:       spec.User,
		Labels: map[string]string{
			labelSandbox:   "1",
			labelName:      spec.Name,
			labelWorkspace: spec.Workspace,
		},
		HostConfig: hostConfig{
			Binds:       []string{spec.Workspace + ":" + spec.Workspace},
			Memory:      int64(spec.MemoryMB) << 20,
			NanoCpus:    int64(spec.CPUs * 1e9),
			NetworkMode: spec.Network,
		},
	}
	path := "/containers/create?name=" + url.QueryEscape(boxPrefix+spec.Name)
	err := d.eng.callJSON(ctx, http.MethodPost, path, body, nil)
	if !isStatus(err, http.StatusNotFound) {
		return err
	}
	if err := d.pull(ctx, spec.Image); err != nil {
		return err
	}
	return d.eng.callJSON(ctx, http.MethodPost, path, body, nil)
}

// pull fetches an image and reads the progress stream to its end, failing on
// any line that carries an "error" field.
func (d *Driver) pull(ctx context.Context, image string) error {
	repo, tag := splitImage(image)
	q := url.Values{"fromImage": {repo}, "tag": {tag}}
	resp, err := d.eng.call(ctx, http.MethodPost, "/images/create?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	for {
		var msg struct {
			Error string `json:"error"`
		}
		err := dec.Decode(&msg)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("docker: read pull of %q: %w", image, err)
		}
		if msg.Error != "" {
			return fmt.Errorf("docker: pull %q: %s", image, msg.Error)
		}
	}
}

// splitImage separates "repo:tag" into its parts. A colon before the last
// slash belongs to a registry port, not a tag. The tag defaults to "latest".
func splitImage(image string) (repo, tag string) {
	if i := strings.LastIndex(image, ":"); i > strings.LastIndex(image, "/") {
		return image[:i], image[i+1:]
	}
	return image, "latest"
}

// listRow is one entry of GET /containers/json.
type listRow struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	State   string            `json:"State"`
	Created int64             `json:"Created"`
	Labels  map[string]string `json:"Labels"`
}

// List returns every sandbox box, running or not.
func (d *Driver) List(ctx context.Context) ([]core.Box, error) {
	filters, err := json.Marshal(map[string][]string{"label": {labelSandbox + "=1"}})
	if err != nil {
		return nil, fmt.Errorf("docker: encode list filters: %w", err)
	}
	q := url.Values{"all": {"1"}, "filters": {string(filters)}}
	var rows []listRow
	if err := d.eng.callJSON(ctx, http.MethodGet, "/containers/json?"+q.Encode(), nil, &rows); err != nil {
		return nil, err
	}
	boxes := make([]core.Box, 0, len(rows))
	for _, r := range rows {
		if len(r.Names) == 0 {
			continue
		}
		boxes = append(boxes, core.Box{
			Name:        strings.TrimPrefix(r.Names[0], "/"+boxPrefix),
			Image:       r.Image,
			Workspace:   r.Labels[labelWorkspace],
			ContainerID: r.ID,
			State:       r.State,
			CreatedAt:   time.Unix(r.Created, 0).UTC(),
		})
	}
	return boxes, nil
}

// Remove force-deletes the box's container, or returns core.ErrNotFound.
func (d *Driver) Remove(ctx context.Context, name string) error {
	err := d.eng.send(ctx, http.MethodDelete, containerPath(name)+"?force=1", nil)
	if isStatus(err, http.StatusNotFound) {
		return notFound(name)
	}
	return err
}

// execCreate is the POST /containers/{name}/exec request.
type execCreate struct {
	AttachStdout bool     `json:"AttachStdout"`
	AttachStderr bool     `json:"AttachStderr"`
	Cmd          []string `json:"Cmd"`
	WorkingDir   string   `json:"WorkingDir"`
}

// execStart is the POST /exec/{id}/start request.
type execStart struct {
	Detach bool `json:"Detach"`
	Tty    bool `json:"Tty"`
}

// Exec runs argv in the box and returns its exit code. Output goes to out.
// On error the exit code is -1.
func (d *Driver) Exec(ctx context.Context, name string, argv []string, workdir string, out io.Writer) (int, error) {
	if out == nil {
		out = io.Discard
	}
	var created struct {
		ID string `json:"Id"`
	}
	err := d.eng.callJSON(ctx, http.MethodPost, containerPath(name)+"/exec", execCreate{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          argv,
		WorkingDir:   workdir,
	}, &created)
	if isStatus(err, http.StatusNotFound) {
		return -1, notFound(name)
	}
	if err != nil {
		return -1, err
	}

	execPath := "/exec/" + url.PathEscape(created.ID)
	resp, err := d.eng.call(ctx, http.MethodPost, execPath+"/start", execStart{})
	if err != nil {
		return -1, err
	}
	err = copyFrames(resp.Body, out)
	resp.Body.Close()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return -1, ctxErr
		}
		return -1, fmt.Errorf("docker: read exec output of %q: %w", name, err)
	}

	var info struct {
		ExitCode int `json:"ExitCode"`
	}
	if err := d.eng.callJSON(ctx, http.MethodGet, execPath+"/json", nil, &info); err != nil {
		return -1, err
	}
	return info.ExitCode, nil
}
