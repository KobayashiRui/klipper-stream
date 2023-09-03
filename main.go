package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"

	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/mediadevices/pkg/frame"

	// _ "github.com/pion/mediadevices/pkg/driver/videotest"
	_ "github.com/pion/mediadevices/pkg/driver/camera" // This is required to register camera adapter
	//_ "github.com/pion/mediadevices/pkg/driver/microphone"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	webcam_url := "127.0.0.1:8080"

	device_info_list := mediadevices.EnumerateDevices()

	select_label := "usb-GG-220402-CX_Depstech_webcam_MIC_01.00.00-video-index0;video0"

	select_id := ""
	for _, d := range device_info_list {
		fmt.Printf("############# Get Device Info #############\n")
		fmt.Printf("DeviceID: %v\n", d.DeviceID)
		fmt.Printf("Label: %v\n", d.Label)
		fmt.Printf("###########################################\n")
		if d.Label == select_label {
			select_id = d.DeviceID
		}
	}

	fmt.Printf("Get Media\n")
	mediaStream, err := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Video: func(constraint *mediadevices.MediaTrackConstraints) {
			constraint.DeviceID = prop.String(select_id)
			constraint.Width = prop.Int(1920)
			constraint.Height = prop.Int(1080)
			constraint.FrameRate = prop.Float(30)
			constraint.FrameFormat = prop.FrameFormat(frame.FormatMJPEG)
		},
	})
	must(err)

	fmt.Printf("Tracks: %d\n", len(mediaStream.GetVideoTracks()))

	track := mediaStream.GetVideoTracks()[0]
	videoTrack := track.(*mediadevices.VideoTrack)
	defer videoTrack.Close()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		action := "stream"
		if get_action := r.URL.Query().Get("action"); get_action != "" {
			action = get_action
		}

		var buf bytes.Buffer
		videoReader := videoTrack.NewReader(false)
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
			encode_option := jpeg.Options{Quality:85}
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

			if action == "snapshot" {
				return
			}
		}
	})

	fmt.Printf("listening on %s\n", webcam_url)
	log.Println(http.ListenAndServe(webcam_url, nil))
}
