package hipchat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
)

const (
	RedBackground    = "red"
	YellowBackground = "yellow"
	GreenBackground  = "green"
	GrayBackground   = "gray"
	RandomBackground = "random"
	PurpleBackground = "purple"
)

type HipchatClient struct {
	RoomId int
	ApiKey string
	HcHost string
	RoHost string
}

func NewClient() *HipchatClient {
	return &HipchatClient{}
}
func (h *HipchatClient) Notify(msg, color string) error {
	if h.ApiKey == "" {
		return errors.New("ApiKey unset")
	}
	msgBody := map[string]interface{}{
		"message": msg,
		"notify":  "true",
		"color":   color,
	}

	body, err := json.Marshal(msgBody)
	if err != nil {
		return err
	}
	roomId := url.QueryEscape(strconv.Itoa(h.RoomId))
	hipchatUrl := fmt.Sprintf("https://%s/v2/room/%s/message?auth_token=%s", h.HcHost, roomId, h.ApiKey)
	req, err := http.NewRequest("POST", hipchatUrl, bytes.NewReader(body))

	req.Header.Add("Content-Type", "application/json")

	Client := http.Client{}
	resp, err := Client.Do(req)
	if err != nil {
		log.Printf("Could not post to hipchat for the reason %s", err.Error())
		return err
	}

	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Could not post to hipchat for the reason %s", err.Error())
		return err
	}
	return nil
}
