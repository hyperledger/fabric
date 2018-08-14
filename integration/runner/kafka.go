/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tedsuo/ifrit"
)

const KafkaDefaultImage = "hyperledger/fabric-kafka:latest"

// Kafka manages the execution of an instance of a dockerized CounchDB
// for tests.
type Kafka struct {
	Client        *docker.Client
	Image         string
	HostIP        string
	HostPort      int
	ContainerPort docker.Port
	Name          string
	NetworkName   string
	StartTimeout  time.Duration

	MessageMaxBytes              int
	ReplicaFetchMaxBytes         int
	UncleanLeaderElectionEnable  bool
	DefaultReplicationFactor     int
	MinInsyncReplicas            int
	BrokerID                     int
	ZooKeeperConnect             string
	ReplicaFetchResponseMaxBytes int
	AdvertisedListeners          string
	LogLevel                     string

	ErrorStream  io.Writer
	OutputStream io.Writer

	ContainerID      string
	HostAddress      string
	ContainerAddress string
	Address          string

	mutex   sync.Mutex
	stopped bool
}

// Run runs a Kafka container. It implements the ifrit.Runner interface
func (k *Kafka) Run(sigCh <-chan os.Signal, ready chan<- struct{}) error {
	if k.Image == "" {
		k.Image = KafkaDefaultImage
	}

	if k.Name == "" {
		k.Name = DefaultNamer()
	}

	if k.HostIP == "" {
		k.HostIP = "127.0.0.1"
	}

	if k.ContainerPort == docker.Port("") {
		k.ContainerPort = docker.Port("9092/tcp")
	}

	if k.StartTimeout == 0 {
		k.StartTimeout = DefaultStartTimeout
	}

	if k.Client == nil {
		client, err := docker.NewClientFromEnv()
		if err != nil {
			return err
		}
		k.Client = client
	}

	if k.DefaultReplicationFactor == 0 {
		k.DefaultReplicationFactor = 1
	}

	if k.MinInsyncReplicas == 0 {
		k.MinInsyncReplicas = 1
	}

	if k.ZooKeeperConnect == "" {
		k.ZooKeeperConnect = "zookeeper:2181/kafka"
	}

	if k.MessageMaxBytes == 0 {
		k.MessageMaxBytes = 1000012
	}

	if k.ReplicaFetchMaxBytes == 0 {
		k.ReplicaFetchMaxBytes = 1048576
	}

	if k.ReplicaFetchResponseMaxBytes == 0 {
		k.ReplicaFetchResponseMaxBytes = 10485760
	}

	if k.LogLevel == "" {
		k.LogLevel = "warn"
	}

	containerOptions := docker.CreateContainerOptions{
		Name: k.Name,
		Config: &docker.Config{
			Image: k.Image,
			Env:   k.buildEnv(),
		},
		HostConfig: &docker.HostConfig{
			AutoRemove: true,
			PortBindings: map[docker.Port][]docker.PortBinding{
				k.ContainerPort: {{
					HostIP:   k.HostIP,
					HostPort: strconv.Itoa(k.HostPort),
				}},
			},
		},
	}

	if k.NetworkName != "" {
		nw, err := k.Client.NetworkInfo(k.NetworkName)
		if err != nil {
			return err
		}

		containerOptions.NetworkingConfig = &docker.NetworkingConfig{
			EndpointsConfig: map[string]*docker.EndpointConfig{
				k.NetworkName: {
					NetworkID: nw.ID,
				},
			},
		}
	}

	container, err := k.Client.CreateContainer(containerOptions)
	if err != nil {
		return err
	}
	k.ContainerID = container.ID

	err = k.Client.StartContainer(container.ID, nil)
	if err != nil {
		return err
	}
	defer k.Stop()

	container, err = k.Client.InspectContainer(container.ID)
	if err != nil {
		return err
	}

	k.HostAddress = net.JoinHostPort(
		container.NetworkSettings.Ports[k.ContainerPort][0].HostIP,
		container.NetworkSettings.Ports[k.ContainerPort][0].HostPort,
	)
	k.ContainerAddress = net.JoinHostPort(
		container.NetworkSettings.Networks[k.NetworkName].IPAddress,
		k.ContainerPort.Port(),
	)

	logContext, cancelLogs := context.WithCancel(context.Background())
	defer cancelLogs()
	go k.streamLogs(logContext)

	containerExit := k.wait()
	ctx, cancel := context.WithTimeout(context.Background(), k.StartTimeout)
	defer cancel()

	select {
	case <-ctx.Done():
		return errors.Wrapf(ctx.Err(), "kafka broker in container %s did not start", k.ContainerID)
	case <-containerExit:
		return errors.New("container exited before ready")
	case <-k.ready(ctx, k.ContainerAddress):
		k.Address = k.ContainerAddress
	case <-k.ready(ctx, k.HostAddress):
		k.Address = k.HostAddress
	}

	cancel()
	close(ready)

	for {
		select {
		case err := <-containerExit:
			return err
		case <-sigCh:
			if err := k.Stop(); err != nil {
				return err
			}
		}
	}
}

func (k *Kafka) buildEnv() []string {
	env := []string{
		"KAFKA_LOG_RETENTION_MS=-1",
		//"KAFKA_AUTO_CREATE_TOPICS_ENABLE=false",
		fmt.Sprintf("KAFKA_MESSAGE_MAX_BYTES=%d", k.MessageMaxBytes),
		fmt.Sprintf("KAFKA_REPLICA_FETCH_MAX_BYTES=%d", k.ReplicaFetchMaxBytes),
		fmt.Sprintf("KAFKA_UNCLEAN_LEADER_ELECTION_ENABLE=%s", strconv.FormatBool(k.UncleanLeaderElectionEnable)),
		fmt.Sprintf("KAFKA_DEFAULT_REPLICATION_FACTOR=%d", k.DefaultReplicationFactor),
		fmt.Sprintf("KAFKA_MIN_INSYNC_REPLICAS=%d", k.MinInsyncReplicas),
		fmt.Sprintf("KAFKA_BROKER_ID=%d", k.BrokerID),
		fmt.Sprintf("KAFKA_ZOOKEEPER_CONNECT=%s", k.ZooKeeperConnect),
		fmt.Sprintf("KAFKA_REPLICA_FETCH_RESPONSE_MAX_BYTES=%d", k.ReplicaFetchResponseMaxBytes),
		fmt.Sprintf("KAFKA_ADVERTISED_LISTENERS=EXTERNAL://localhost:%d,%s://%s:9093", k.HostPort, k.NetworkName, k.Name),
		fmt.Sprintf("KAFKA_LISTENERS=EXTERNAL://0.0.0.0:9092,%s://0.0.0.0:9093", k.NetworkName),
		fmt.Sprintf("KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=EXTERNAL:PLAINTEXT,%s:PLAINTEXT", k.NetworkName),
		fmt.Sprintf("KAFKA_INTER_BROKER_LISTENER_NAME=%s", k.NetworkName),
	}
	return env
}

func (k *Kafka) ready(ctx context.Context, addr string) <-chan struct{} {
	readyCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
			if err == nil {
				conn.Close()
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

func (k *Kafka) wait() <-chan error {
	exitCh := make(chan error)
	go func() {
		exitCode, err := k.Client.WaitContainer(k.ContainerID)
		if err == nil {
			err = fmt.Errorf("kafka: process exited with %d", exitCode)
		}
		exitCh <- err
	}()

	return exitCh
}

func (k *Kafka) streamLogs(ctx context.Context) error {
	if k.ErrorStream == nil && k.OutputStream == nil {
		return nil
	}

	logOptions := docker.LogsOptions{
		Context:      ctx,
		Container:    k.ContainerID,
		ErrorStream:  k.ErrorStream,
		OutputStream: k.OutputStream,
		Stderr:       k.ErrorStream != nil,
		Stdout:       k.OutputStream != nil,
		Follow:       true,
	}
	return k.Client.Logs(logOptions)
}

// Start starts the Kafka container using an ifrit runner
func (k *Kafka) Start() error {
	p := ifrit.Invoke(k)

	select {
	case <-p.Ready():
		return nil
	case err := <-p.Wait():
		return err
	}
}

// Stop stops and removes the Kafka container
func (k *Kafka) Stop() error {
	k.mutex.Lock()
	if k.stopped {
		k.mutex.Unlock()
		return errors.Errorf("container %s already stopped", k.ContainerID)
	}
	k.stopped = true
	k.mutex.Unlock()

	return k.Client.StopContainer(k.ContainerID, 0)
}
