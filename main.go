package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strings"

	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/prop"

	// _ "github.com/pion/mediadevices/pkg/driver/videotest"
	_ "github.com/pion/mediadevices/pkg/driver/camera" // This is required to register camera adapter
	//_ "github.com/pion/mediadevices/pkg/driver/microphone"

	"github.com/go-ini/ini"
)

type CameraInfo struct {
	port      string
	device    string
	device_id string
	width     int
	height    int
}

func initCameraInfo(port string, device string, width, height int) CameraInfo {
	return CameraInfo{port, device, "", width, height}
}

type CameraInfoList []CameraInfo

func (c CameraInfoList) addCameraInfo(ci CameraInfo) CameraInfoList {
	return append(c, ci)
}

// CameraInfo内のdeviceに引数としていれたdeviceがs存在sしているかの判定
func (c CameraInfoList) setCameraDeviceId(d mediadevices.MediaDeviceInfo) {
	for i, v := range c {
		//TODO v.deviceが空文字の場合の対処
		if v.device == d.Label {
			c[i].device_id = d.DeviceID
		}
	}
}

func SetCameraServer(info CameraInfo) {
	mediaStream, err := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Video: func(constraint *mediadevices.MediaTrackConstraints) {
			//TODO: check params
			constraint.DeviceID = prop.String(info.device_id)
			constraint.Width = prop.Int(info.width)
			constraint.Height = prop.Int(info.height)
			constraint.FrameRate = prop.Float(30)
			constraint.FrameFormat = prop.FrameFormat(frame.FormatMJPEG)
		},
	})
	must(err)
	fmt.Printf("Tracks: %d\n", len(mediaStream.GetVideoTracks()))
	track := mediaStream.GetVideoTracks()[0]
	videoTrack := track.(*mediadevices.VideoTrack)
	defer videoTrack.Close()

	serverMuxCamera := http.NewServeMux()

	serverMuxCamera.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		action := "stream"
		if get_action := r.URL.Query().Get("action"); get_action != "" {
			action = get_action
		}

		var buf bytes.Buffer
		videoReader := videoTrack.NewReader(false)
		w.Header().Add("Access-Control-Allow-Credentials", "true")
		w.Header().Add("Access-Control-Allow-Origin", "*")
		cacheControl := "no-stroe, no-cache, must-revalidate, proxy-revalidate, pre-check=0, post-check=0, max-age-0"
		w.Header().Add("Cache-Control", cacheControl)
		w.Header().Add("Pragma", "no-cache")
		w.Header().Add("Connection", "keep-alive")

		if action == "snapshot" {
			contentType := "image/jpeg"
			w.Header().Add("Content-Type", contentType)
			frame, release, err := videoReader.Read()
			if err == io.EOF {
				return
			}
			must(err)

			//err = jpeg.Encode(&buf, frame, nil)
			encode_option := jpeg.Options{Quality: 85}
			err = jpeg.Encode(&buf, frame, &encode_option)
			// Since we're done with img, we need to release img so that that the original owner can reuse
			// this memory.
			release()
			must(err)

			_, err = w.Write(buf.Bytes())
			buf.Reset()
			must(err)
		} else {
			mimeWriter := multipart.NewWriter(w)
			contentType := fmt.Sprintf("multipart/x-mixed-replace;boundary=%s", mimeWriter.Boundary())
			w.Header().Add("Content-Type", contentType)

			partHeader := make(textproto.MIMEHeader)
			partHeader.Add("Content-Type", "image/jpeg")
			//partHeader.Add("Content-Type", "video/x-jpeg")

			for {
				frame, release, err := videoReader.Read()
				if err == io.EOF {
					return
				}
				must(err)

				//err = jpeg.Encode(&buf, frame, nil)
				encode_option := jpeg.Options{Quality: 85}
				err = jpeg.Encode(&buf, frame, &encode_option)
				// Since we're done with img, we need to release img so that that the original owner can reuse
				// this memory.
				release()
				must(err)

				partWriter, err := mimeWriter.CreatePart(partHeader)
				must(err)

				_, err = partWriter.Write(buf.Bytes())
				buf.Reset()
				must(err)
			}
		}
	})
	http.ListenAndServe("127.0.0.1:"+info.port, serverMuxCamera)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {

	if len(os.Args) != 2 {
		fmt.Printf("plz set config file path\n")
		return
	}
	config_file_path := os.Args[1]

	cfg, cfg_err := ini.LoadSources(ini.LoadOptions{
		KeyValueDelimiters: ":",
	}, config_file_path)

	if cfg_err != nil {
		return
	}

	//val := cfg.Section("klipper-stream").Key("test").String()
	//fmt.Printf("config: %s \n", val)

	//klipper-streamの設定を読み込む
	ks_camera_list_port := "8090"
	if cfg.Section("klipper-stream").HasValue("port") {
		ks_camera_list_port = cfg.Section("klipper-stream").Key("port").String()
	}

	//cameraの設定読み込み
	var camera_info_list CameraInfoList
	cam_index := 1
	for cam_index <= 10 {
		sec, err := cfg.GetSection(fmt.Sprintf("cam %d", cam_index))
		if sec == nil || err != nil {
			break
		}

		_cam_port := fmt.Sprint((8080 + cam_index - 1))
		if sec.HasKey("port") {
			_cam_port = sec.Key("port").String()
		}

		_cam_device := ""
		if sec.HasKey("device") {
			_cam_device = sec.Key("device").String()
		}

		_width := 0
		if sec.HasKey("width") {
			_width = sec.Key("width").MustInt()
		}

		_height := 0
		if sec.HasKey("height") {
			_height = sec.Key("height").MustInt()
		}

		fmt.Printf("cam device: %s, cam port: %s\n", _cam_port, _cam_device)
		camera_info_list = camera_info_list.addCameraInfo((initCameraInfo(_cam_port, _cam_device, _width, _height)))

		cam_index += 1
		//val := cfg.Section("klipper-stream").Key("cam "+ cam_index).String()
	}

	device_info_list := mediadevices.EnumerateDevices()

	//select_label := "usb-GG-220402-CX_Depstech_webcam_MIC_01.00.00-video-index0;video0"
	//select_label := os.Args[1]

	var labels []string

	//select_id := ""
	for _, d := range device_info_list {
		fmt.Printf("############# Get Device Info #############\n")
		fmt.Printf("DeviceID: %v\n", d.DeviceID)
		fmt.Printf("Label: %v\n", d.Label)
		fmt.Printf("###########################################\n")
		camera_info_list.setCameraDeviceId(d)
		labels = append(labels, d.Label)
	}

	//Loop camera deivce List
	for _, info := range camera_info_list {
		go SetCameraServer(info)
	}

	serverMuxCameraList := http.NewServeMux()
	serverMuxCameraList.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		res := strings.Join(labels, "\n")

		fmt.Fprintf(w, res)
	})

	http.ListenAndServe("127.0.0.1:"+ks_camera_list_port, serverMuxCameraList)
}
