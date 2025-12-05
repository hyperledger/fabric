/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	dcontainer "github.com/moby/moby/api/types/container"
	dnetwork "github.com/moby/moby/api/types/network"
	dcli "github.com/moby/moby/client"
	"github.com/pkg/errors"
	"github.com/tedsuo/ifrit"
)

const (
	CouchDBDefaultImage = "couchdb:3.4.2"
	CouchDBUsername     = "admin"
	CouchDBPassword     = "adminpw"
)

// CouchDB manages the execution of an instance of a dockerized CounchDB
// for tests.
type CouchDB struct {
	Client        dcli.APIClient
	Image         string
	HostIP        string
	HostPort      int
	ContainerPort dnetwork.Port
	Name          string
	StartTimeout  time.Duration
	Binds         []string

	OutputStream io.Writer

	creator          string
	containerID      string
	hostAddress      string
	containerAddress string
	address          string

	mutex   sync.Mutex
	stopped bool
}

// Run runs a CouchDB container. It implements the ifrit.Runner interface
func (c *CouchDB) Run(sigCh <-chan os.Signal, ready chan<- struct{}) error {
	if c.Image == "" {
		c.Image = CouchDBDefaultImage
	}

	if c.Name == "" {
		c.Name = DefaultNamer()
	}

	if c.HostIP == "" {
		c.HostIP = "127.0.0.1"
	}

	if c.ContainerPort.IsZero() || c.ContainerPort.Num() == 0 {
		c.ContainerPort = dnetwork.MustParsePort("5984/tcp")
	}

	if c.StartTimeout == 0 {
		c.StartTimeout = DefaultStartTimeout
	}

	if c.Client == nil {
		client, err := dcli.New(dcli.FromEnv)
		if err != nil {
			return err
		}
		c.Client = client
	}

	hostConfig := &dcontainer.HostConfig{
		Binds: c.Binds,
		PortBindings: map[dnetwork.Port][]dnetwork.PortBinding{
			c.ContainerPort: {{
				HostIP:   netip.MustParseAddr(c.HostIP),
				HostPort: strconv.Itoa(c.HostPort),
			}},
		},
		AutoRemove: true,
	}

	container, err := c.Client.ContainerCreate(context.Background(), dcli.ContainerCreateOptions{
		Config: &dcontainer.Config{
			Env: []string{
				fmt.Sprintf("_creator=%s", c.creator),
				fmt.Sprintf("COUCHDB_USER=%s", CouchDBUsername),
				fmt.Sprintf("COUCHDB_PASSWORD=%s", CouchDBPassword),
			},
			Image: c.Image,
		},
		HostConfig: hostConfig,
		Name:       c.Name,
	})
	if err != nil {
		return err
	}
	c.containerID = container.ID

	_, err = c.Client.ContainerStart(context.Background(), container.ID, dcli.ContainerStartOptions{})
	if err != nil {
		return err
	}
	defer c.Stop()

	res, err := c.Client.ContainerInspect(context.Background(), container.ID, dcli.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	c.hostAddress = net.JoinHostPort(
		res.Container.NetworkSettings.Ports[c.ContainerPort][0].HostIP.String(),
		res.Container.NetworkSettings.Ports[c.ContainerPort][0].HostPort,
	)
	c.containerAddress = net.JoinHostPort(
		res.Container.NetworkSettings.Networks[res.Container.HostConfig.NetworkMode.NetworkName()].IPAddress.String(),
		strconv.Itoa(int(c.ContainerPort.Num())),
	)

	streamCtx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()
	go c.streamLogs(streamCtx)

	containerExit := c.wait()
	ctx, cancel := context.WithTimeout(context.Background(), c.StartTimeout)
	defer cancel()

	select {
	case <-ctx.Done():
		return errors.Wrapf(ctx.Err(), "database in container %s did not start", c.containerID)
	case <-containerExit:
		return errors.New("container exited before ready")
	case <-c.ready(ctx, c.hostAddress):
		c.address = c.hostAddress
	case <-c.ready(ctx, c.containerAddress):
		c.address = c.containerAddress
	}

	cancel()
	close(ready)

	for {
		select {
		case err := <-containerExit:
			return err
		case <-sigCh:
			if err := c.Stop(); err != nil {
				return err
			}
		}
	}
}

func endpointReady(ctx context.Context, url string) bool {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false
	}

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	return err == nil && resp.StatusCode == http.StatusOK
}

func (c *CouchDB) ready(ctx context.Context, addr string) <-chan struct{} {
	readyCh := make(chan struct{})
	go func() {
		url := fmt.Sprintf("http://%s:%s@%s/", CouchDBUsername, CouchDBPassword, addr)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			if endpointReady(ctx, url) {
				close(readyCh)
				return
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()

	return readyCh
}

func (c *CouchDB) wait() <-chan error {
	exitCh := make(chan error)
	go func() {
		resWait := c.Client.ContainerWait(context.Background(), c.containerID, dcli.ContainerWaitOptions{})
		select {
		case res := <-resWait.Result:
			err := fmt.Errorf("couchdb: process exited with %d", res.StatusCode)
			exitCh <- err
		case err := <-resWait.Error:
			exitCh <- err
		}
	}()

	return exitCh
}

func (c *CouchDB) streamLogs(ctx context.Context) {
	if c.OutputStream == nil {
		return
	}

	go func() {
		res, err := c.Client.ContainerLogs(ctx, c.containerID, dcli.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
		})
		if err != nil {
			fmt.Fprintf(c.OutputStream, "log stream ended with error: %s", err)
		}
		defer res.Close()

		reader := bufio.NewReader(res)

		for {
			select {
			case <-ctx.Done():
				fmt.Fprint(c.OutputStream, "log stream ended with cancel context")
				return
			default:
				// Loop forever dumping lines of text into the containerLogger
				// until the pipe is closed
				line, err := reader.ReadString('\n')
				if len(line) > 0 {
					c.OutputStream.Write([]byte(line))
				}
				switch err {
				case nil:
				case io.EOF:
					fmt.Fprintf(c.OutputStream, "Container %s has closed its IO channel", c.containerID)
					return
				default:
					fmt.Fprintf(c.OutputStream, "Error reading container output: %s", err)
					return
				}
			}
		}
	}()
}

// Address returns the address successfully used by the readiness check.
func (c *CouchDB) Address() string {
	return c.address
}

// HostAddress returns the host address where this CouchDB instance is available.
func (c *CouchDB) HostAddress() string {
	return c.hostAddress
}

// ContainerAddress returns the container address where this CouchDB instance
// is available.
func (c *CouchDB) ContainerAddress() string {
	return c.containerAddress
}

// ContainerID returns the container ID of this CouchDB
func (c *CouchDB) ContainerID() string {
	return c.containerID
}

// Start starts the CouchDB container using an ifrit runner
func (c *CouchDB) Start() error {
	c.creator = string(debug.Stack())
	p := ifrit.Invoke(c)

	select {
	case <-p.Ready():
		return nil
	case err := <-p.Wait():
		return err
	}
}

// Stop stops and removes the CouchDB container
func (c *CouchDB) Stop() error {
	c.mutex.Lock()
	if c.stopped {
		c.mutex.Unlock()
		return errors.Errorf("container %s already stopped", c.containerID)
	}
	c.stopped = true
	c.mutex.Unlock()

	t := 0
	_, err := c.Client.ContainerStop(context.Background(), c.containerID, dcli.ContainerStopOptions{
		Timeout: &t,
	})
	if err != nil {
		return err
	}

	return nil
}
