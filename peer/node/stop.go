/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package node

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"syscall"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/peer"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
)

func stopCmd() *cobra.Command {
	nodeStopCmd.Flags().StringVar(&stopPidFile, "stop-peer-pid-file",
		viper.GetString("peer.fileSystemPath"),
		"Location of peer pid local file, for forces kill")

	return nodeStopCmd
}

var nodeStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stops the running node.",
	Long:  `Stops the running node, disconnecting from the network.`,
	Run: func(cmd *cobra.Command, args []string) {
		stop()
	},
}

func stop() (err error) {
	clientConn, err := peer.NewPeerClientConnection()
	if err != nil {
		pidFile := stopPidFile + "/peer.pid"
		//fmt.Printf("Stopping local peer using process pid from %s \n", pidFile)
		logger.Infof("Error trying to connect to local peer: %s", err)
		logger.Infof("Stopping local peer using process pid from %s", pidFile)
		pid, ferr := readPid(pidFile)
		if ferr != nil {
			err = fmt.Errorf("Error trying to read pid from %s: %s", pidFile, ferr)
			return
		}
		killerr := syscall.Kill(pid, syscall.SIGTERM)
		if killerr != nil {
			err = fmt.Errorf("Error trying to kill -9 pid %d: %s", pid, killerr)
			return
		}
		return nil
	}
	logger.Info("Stopping peer using grpc")
	serverClient := pb.NewAdminClient(clientConn)

	status, err := serverClient.StopServer(context.Background(), &empty.Empty{})
	db.Stop()
	if err != nil {
		fmt.Println(&pb.ServerStatus{Status: pb.ServerStatus_STOPPED})
		return nil
	}

	err = fmt.Errorf("Connection remain opened, peer process doesn't exit")
	fmt.Println(status)
	return err
}

func readPid(fileName string) (int, error) {
	fd, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return 0, err
	}
	defer fd.Close()
	if err := syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return 0, fmt.Errorf("can't lock '%s', lock is held", fd.Name())
	}

	if _, err := fd.Seek(0, 0); err != nil {
		return 0, err
	}

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(string(bytes.TrimSpace(data)))
	if err != nil {
		return 0, fmt.Errorf("error parsing pid from %s: %s", fd.Name(), err)
	}

	if err := syscall.Flock(int(fd.Fd()), syscall.LOCK_UN); err != nil {
		return 0, fmt.Errorf("can't release lock '%s', lock is held", fd.Name())
	}

	return pid, nil

}
