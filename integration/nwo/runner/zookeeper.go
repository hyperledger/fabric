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
	"os"
	"strconv"
	"sync"
	"time"

	dcontainer "github.com/moby/moby/api/types/container"
	dnetwork "github.com/moby/moby/api/types/network"
	dcli "github.com/moby/moby/client"
	"github.com/pkg/errors"
	"github.com/tedsuo/ifrit"
)

const ZooKeeperDefaultImage = "confluentinc/cp-zookeeper:5.3.1"

type ZooKeeper struct {
	Client         dcli.APIClient
	Image          string
	HostIP         string
	HostPort       []int
	ContainerPorts []dnetwork.Port
	Name           string
	StartTimeout   time.Duration

	NetworkName string
	ClientPort  dnetwork.Port
	LeaderPort  dnetwork.Port
	PeerPort    dnetwork.Port
	ZooMyID     int
	ZooServers  string

	OutputStream io.Writer

	containerID      string
	containerAddress string
	address          string

	mutex   sync.Mutex
	stopped bool
}

func (z *ZooKeeper) Run(sigCh <-chan os.Signal, ready chan<- struct{}) error {
	if z.Image == "" {
		z.Image = ZooKeeperDefaultImage
	}

	if z.Name == "" {
		z.Name = DefaultNamer()
	}

	if z.HostIP == "" {
		z.HostIP = "127.0.0.1"
	}

	if z.ContainerPorts == nil {
		if z.ClientPort.IsZero() || z.ClientPort.Num() == 0 {
			z.ClientPort = dnetwork.MustParsePort("2181/tcp")
		}
		if z.LeaderPort.IsZero() || z.LeaderPort.Num() == 0 {
			z.LeaderPort = dnetwork.MustParsePort("3888/tcp")
		}
		if z.PeerPort.IsZero() || z.PeerPort.Num() == 0 {
			z.PeerPort = dnetwork.MustParsePort("2888/tcp")
		}

		z.ContainerPorts = []dnetwork.Port{
			z.ClientPort,
			z.LeaderPort,
			z.PeerPort,
		}
	}

	if z.StartTimeout == 0 {
		z.StartTimeout = DefaultStartTimeout
	}

	if z.ZooMyID == 0 {
		z.ZooMyID = 1
	}

	if z.Client == nil {
		client, err := dcli.New(dcli.FromEnv)
		if err != nil {
			return err
		}
		z.Client = client
	}

	containerOptions := dcli.ContainerCreateOptions{
		Name: z.Name,
		HostConfig: &dcontainer.HostConfig{
			AutoRemove: true,
		},
		Config: &dcontainer.Config{
			Image: z.Image,
			Env: []string{
				fmt.Sprintf("ZOOKEEPER_MY_ID=%d", z.ZooMyID),
				fmt.Sprintf("ZOOKEEPER_SERVERS=%s", z.ZooServers),
				fmt.Sprintf("ZOOKEEPER_CLIENT_PORT=%d", z.ClientPort.Num()),
			},
		},
	}

	if z.NetworkName != "" {
		nw, err := z.Client.NetworkInspect(context.Background(), z.NetworkName, dcli.NetworkInspectOptions{})
		if err != nil {
			return err
		}

		containerOptions.NetworkingConfig = &dnetwork.NetworkingConfig{
			EndpointsConfig: map[string]*dnetwork.EndpointSettings{
				z.NetworkName: {
					NetworkID: nw.Network.ID,
				},
			},
		}
	}

	container, err := z.Client.ContainerCreate(context.Background(), containerOptions)
	if err != nil {
		return err
	}
	z.containerID = container.ID

	_, err = z.Client.ContainerStart(context.Background(), container.ID, dcli.ContainerStartOptions{})
	if err != nil {
		return err
	}
	defer z.Stop()

	res, err := z.Client.ContainerInspect(context.Background(), container.ID, dcli.ContainerInspectOptions{})
	if err != nil {
		return err
	}

	z.containerAddress = net.JoinHostPort(
		res.Container.NetworkSettings.Networks[res.Container.HostConfig.NetworkMode.NetworkName()].IPAddress.String(),
		strconv.Itoa(int(z.ContainerPorts[0].Num())),
	)

	streamCtx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()
	go z.streamLogs(streamCtx)

	containerExit := z.wait()
	ctx, cancel := context.WithTimeout(context.Background(), z.StartTimeout)
	defer cancel()

	select {
	case <-ctx.Done():
		return errors.Wrapf(ctx.Err(), "zookeeper in container %s did not start", z.containerID)
	case <-containerExit:
		return errors.New("container exited before ready")
	default:
		z.address = z.containerAddress
	}

	close(ready)

	for {
		select {
		case err = <-containerExit:
			return err
		case <-sigCh:
			if err = z.Stop(); err != nil {
				return err
			}
		}
	}
}

func (z *ZooKeeper) wait() <-chan error {
	exitCh := make(chan error)
	go func() {
		resWait := z.Client.ContainerWait(context.Background(), z.containerID, dcli.ContainerWaitOptions{})
		select {
		case res := <-resWait.Result:
			err := fmt.Errorf("zookeeper: process exited with %d", res.StatusCode)
			exitCh <- err
		case err := <-resWait.Error:
			exitCh <- err
		}
	}()

	return exitCh
}

func (z *ZooKeeper) streamLogs(ctx context.Context) {
	if z.OutputStream == nil {
		return
	}

	res, err := z.Client.ContainerLogs(ctx, z.containerID, dcli.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		fmt.Fprintf(z.OutputStream, "log stream ended with error: %s", err)
	}
	defer res.Close()

	reader := bufio.NewReader(res)

	for {
		select {
		case <-ctx.Done():
			fmt.Fprint(z.OutputStream, "log stream ended with cancel context")
			return
		default:
			// Loop forever dumping lines of text into the containerLogger
			// until the pipe is closed
			line, err := reader.ReadString('\n')
			if len(line) > 0 {
				z.OutputStream.Write([]byte(line))
			}
			switch err {
			case nil:
			case io.EOF:
				fmt.Fprintf(z.OutputStream, "Container %s has closed its IO channel", z.containerID)
				return
			default:
				fmt.Fprintf(z.OutputStream, "Error reading container output: %s", err)
				return
			}
		}
	}
}

func (z *ZooKeeper) ContainerID() string {
	return z.containerID
}

func (z *ZooKeeper) ContainerAddress() string {
	return z.containerAddress
}

func (z *ZooKeeper) Start() error {
	p := ifrit.Invoke(z)

	select {
	case <-p.Ready():
		return nil
	case err := <-p.Wait():
		return err
	}
}

func (z *ZooKeeper) Stop() error {
	z.mutex.Lock()
	if z.stopped {
		z.mutex.Unlock()
		return errors.Errorf("container %s already stopped", z.Name)
	}
	z.stopped = true
	z.mutex.Unlock()

	t := 0
	_, err := z.Client.ContainerStop(context.Background(), z.containerID, dcli.ContainerStopOptions{
		Timeout: &t,
	})
	if err != nil {
		return err
	}

	_, err = z.Client.VolumePrune(context.Background(), dcli.VolumePruneOptions{
		All:     false,
		Filters: nil,
	})

	return err
}
