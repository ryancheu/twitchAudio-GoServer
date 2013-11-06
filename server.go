package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"
)

type QualityType struct {
	Play    string
	Token   string
	Connect string `"json:"connect"`
	Bitrate float64
}

type StreamInfo struct {
	LastAccess time.Time
	Command    *exec.Cmd
	Port       int
}

//Snippet for json responses taken from
//http://nesv.blogspot.com/2012/09/super-easy-json-http-responses-in-go.html
type Response map[string]interface{}

func (r Response) String() (s string) {
	b, err := json.Marshal(r)
	if err != nil {
		s = ""
		return
	}
	s = string(b)
	return
}

const startPort = 6000
const maxPorts = 1000
const queryPort = 8080
const refreshPeriod = 600 //In minutes

var streams map[string]StreamInfo
var curPort = startPort

func viewHandler(rw http.ResponseWriter, r *http.Request) {
	username := r.URL.Path[len("/view/"):]
	port, success := getPortForUsername(username)

	rw.Header().Set("Content-Type", "application/json")
	rw.Header().Set("Access-Control-Allow-Origin", "*")
	if success {
		fmt.Fprint(rw, Response{"port": port})
	} else {
		fmt.Fprint(rw, Response{"port": port})
	}

}

func main() {
	streams = make(map[string]StreamInfo)
	http.HandleFunc("/view/", viewHandler)
	http.ListenAndServe(":"+strconv.Itoa(queryPort), nil)
}

//Port, success
func getPortForUsername(username string) (int, bool) {
	streamData, suc := streams[username]
	if !suc || checkShouldRefresh(streamData) {
		qual := getStreamData(username)
		if qual != nil {
			//TODO: Make this use a heap to reuse old stream ports
			curPort += 1
			var cmd *exec.Cmd
			if !suc {
				cmd = startVLCStream(*qual, curPort)
			} else {
				//Reuse old port if there is one
				cmd = startVLCStream(*qual, streamData.Port)
			}

			streamData = StreamInfo{LastAccess: time.Now(), Command: cmd, Port: curPort}
			streams[username] = streamData
			return streamData.Port, true
		} else {
			//Error getting stream data from Twitch
			return -1, false
		}
	}

	return streamData.Port, true

}

func checkShouldRefresh(sInfo StreamInfo) bool {
	needsTimeoutRefresh := time.Since(sInfo.LastAccess).Minutes() > refreshPeriod
	streamDied := false

	//Process state occassionaly throws a nil pointer exception so do this instead
	processID := sInfo.Command.Process.Pid
	p, err := os.FindProcess(processID)
	if err != nil || p == nil {
		streamDied = true
	}
	return streamDied || needsTimeoutRefresh
}

func handleError(err error) {
	fmt.Println("error: ")
	fmt.Println(err)
}

func getStreamData(username string) *QualityType {
	url := "http://usher.justin.tv/find/" + username + ".json?type=any"
	res, err := http.Get(url)
	if err != nil {
		handleError(err)
		return nil
	}

	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		handleError(err)
		return nil
	}

	var jsonResult []QualityType
	err = json.Unmarshal(body, &jsonResult)

	//Find lowest bitrate stream
	lowestBitrate := 100000.0
	var lowestQuality QualityType
	var quality QualityType
	for qualInd := range jsonResult {
		quality = jsonResult[qualInd]
		if quality.Bitrate < lowestBitrate {
			lowestBitrate = quality.Bitrate
			lowestQuality = quality
		}
	}
	if lowestBitrate > 5000 {
		return nil
	}
	return &lowestQuality
}

func startVLCStream(streamData QualityType, port int) *exec.Cmd {
	var buffer bytes.Buffer

	buffer.WriteString("rtmpdump --live -r '")
	buffer.WriteString(streamData.Connect)
	buffer.WriteString("' -W 'http://www-cdn.jtvnw.net/widgets/live_site_player.swf' -p 'http://www.twitch.tv/' --jtv '")
	buffer.WriteString(streamData.Token)
	buffer.WriteString("' --playpath '")
	buffer.WriteString(streamData.Play)
	buffer.WriteString("' --quiet --flv '-'")
	buffer.WriteString("| vlc --intf=dummy --play-and-exit --rc-fake-tty -vvv - --sout '")
	buffer.WriteString("#transcode{vcodec=none,acodec=mp3,ab=72k}:standard{access=http,mux=ts,dst=:")
	buffer.WriteString(strconv.Itoa(port))
	buffer.WriteString("}'")

	command := exec.Command("bash", "-c", buffer.String())
	fmt.Println("Starting VLC stream")
	go command.Run()

	fmt.Println("returning out of vlc function")
	return command
}
