// https://10mb.video was down, so...
// Fit a video into a 10mb file (Discord nitro pls?)
//
// Needs ffmpeg ffprobe fdkaac
// Tested only on Linux/Termux
//
// To build this you need the Go compiler:
// go build -o 10mb.video main.go
//
// Caveats:
// ffmpeg doesn't like hi-res square still image videos (youtube music stuff),
// the encoder will overshoot the bitrate by a huge margin in such case
// to work around you may try "-down 512"
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
)

const USAGE = `Usage: %s [OPTIONS] [FILE]

Compress a video to target size
(default audio: 32kbps stereo he-aac v2)

Options:
-down float
	  resolution downscale multiplier (default 1)
	  values above 100 scales by the width in pixels
-music
	  64kbps stereo audio (he-aac v1)
-voice
	  16kbps mono audio (he-aac v1)
-mute
	  no audio
-preset string
	  h264 encode preset (default "slow")
-size float
	  target size in MB (default 10)
`

func main() {
	// check for dependencies
	exes := []string{"ffmpeg", "ffprobe", "fdkaac"}
	for _, exe := range exes {
		if _, err := exec.LookPath(exe); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	size := flag.Float64("size", 10, "target size in MB")
	preset := flag.String("preset", "slow", "h264 encode preset")
	down := flag.Float64("down", 1, "resolution downscale multiplier, "+
		"values above 100 scales by the width in pixels")
	music := flag.Bool("music", false, "64kbps stereo audio (he-aac v1)")
	voice := flag.Bool("voice", false, "16kbps mono audio (he-aac v1)")
	mute := flag.Bool("mute", false, "no audio")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, USAGE, path.Base(os.Args[0]))
	}
	flag.Parse()

	if len(flag.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "error: no filename given")
		os.Exit(1)
	}
	if *down < 1 {
		fmt.Fprintln(os.Stderr, "downscale multiplier cannot be less than 1")
		os.Exit(1)
	}

	filepath := flag.Args()[0]
	file := path.Base(filepath)
	dir := path.Dir(filepath)

	err := os.Chdir(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// get video lenght in seconds
	probe := exec.Command(
		"ffprobe",
		"-i", file,
		"-show_entries", "format=duration",
		"-v", "quiet",
		"-of", "csv=p=0",
	)

	secbytes, err := probe.Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	secbytes = bytes.TrimSpace(secbytes)
	seconds, err := strconv.ParseFloat(string(secbytes), 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	const MEG = 8388.608 // https://trac.ffmpeg.org/wiki/Encode/H.264#twopass
	bitfloat := *size * MEG / seconds

	// ffmpeg encodes stuff in chunks
	// we need to deal with possible bitrate overshoot
	// guessed values
	// switch {
	// case bitfloat > 800:
	// 	// 256KB overshoot
	// 	bitfloat -= 0.25 * MEG / seconds
	// case bitfloat > 400:
	// 	// 64KB overshoot
	// 	bitfloat -= 0.0625 * MEG / seconds
	// default:
	// 	// 32KB overshoot
	// 	bitfloat -= 0.03125 * MEG / seconds
	//}
	bitfloat -= 0.25 * MEG / seconds

	// muxing overhead (not exact science)
	// based on observed values
	// depends on fps and who knows...
	overhead := 86.8 / bitfloat * 0.05785312
	bitfloat -= bitfloat * overhead

	// allocate cmds
	wavfile := &exec.Cmd{}
	aacfile := &exec.Cmd{}
	pass1 := &exec.Cmd{}
	pass2 := &exec.Cmd{}

	// trap ctrl+c and kill
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		if pass1.Process != nil {
			pass1.Process.Kill()
		}
		if wavfile.Process != nil {
			wavfile.Process.Kill()
		}
		if aacfile.Process != nil {
			aacfile.Process.Kill()
		}
		if pass2.Process != nil {
			pass2.Process.Kill()
		}
	}()

	// remove tmp files
	// we don't use /tmp because Termux and Android
	cleanup := func() {
		os.Remove(file + "-0.log")
		os.Remove(file + "-0.log.mbtree")
		os.Remove(file + "-0.log.temp")
		os.Remove(file + "-0.log.mbtree.temp")
		os.Remove(file + ".wav")
		os.Remove(file + ".m4a")
	}

	abitrate := 32
	audioch := 2
	profile := "29" // HE-AACv2
	if *music {
		abitrate = 64
		profile = "5" // HE-AACv1
	}
	if *voice {
		abitrate = 16
		audioch = 1
		profile = "5" // HE-AACv1
	}

	// we need to do this mumbo jumbo because fdk_aac encoder is disabled
	// on 99.99% of ffmpeg installations (even on Arch)
	// fdkaac standalone encoder is fine though
	//
	// wav decode
	wavfile = exec.Command(
		"ffmpeg", "-y",
		"-i", file,
		"-ar", "44100",
		"-ac", fmt.Sprintf("%d", audioch),
		file+".wav",
	)
	wavfile.Stderr = os.Stderr
	wavfile.Stdout = os.Stdout
	if !*mute {
		err = wavfile.Run()
		if err != nil {
			// catch kill signals
			if strings.Contains(err.Error(), "signal") {
				cleanup()
				os.Exit(1)
			} else {
				// if wav decode fails, there's no audio
				*mute = true
			}

		}
	}
	// aac encode
	aacfile = exec.Command(
		"fdkaac",
		"-p", profile,
		"-b", fmt.Sprintf("%d000", abitrate),
		file+".wav",
	)
	aacfile.Stderr = os.Stderr
	aacfile.Stdout = os.Stdout
	if !*mute {
		err = aacfile.Run()
		if err != nil {
			cleanup()
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	// video bitrate
	vbitrate := int(bitfloat) - abitrate
	if *mute {
		vbitrate = int(bitfloat)
	}

	// construct output filename
	arr := strings.Split(file, ".")
	output := strings.Join(arr[0:len(arr)-1], ".")
	output = fmt.Sprintf("%gmb.%s.mp4", *size, output)

	// beware: changing this changes the muxing overhead
	const FPS = 24

	// resolution scale/crop filter and FPS
	// because h264 wants to be multiples of 2
	vfparams := ":force_original_aspect_ratio=increase," +
		"setsar=1," +
		"crop=trunc(iw/2)*2:trunc(ih/2)*2," +
		"fps=%d"

	vfopt := fmt.Sprintf(
		"scale=iw/%f:-1"+vfparams, *down, FPS,
	)

	if *down >= 100 {
		vfopt = fmt.Sprintf(
			"scale=%f:-1"+vfparams, *down, FPS,
		)
	}

	// pass 1
	pass1 = exec.Command(
		"ffmpeg", "-y",
		"-i", file,
		//"-fs", fmt.Sprintf("%gM", *size),
		"-vf", vfopt,
		"-c:v", "libx264",
		"-preset", *preset,
		"-b:v", fmt.Sprintf("%dk", vbitrate),
		"-pass", "1",
		"-passlogfile", file,
		"-movflags", "+faststart",
		"-an",
		"-f", "null",
		"/dev/null",
	)
	pass1.Stderr = os.Stderr
	pass1.Stdout = os.Stdout
	err = pass1.Run()
	if err != nil {
		cleanup()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// pass 2
	if *mute {
		pass2 = exec.Command(
			"ffmpeg", "-y",
			"-i", file,
			//"-fs", fmt.Sprintf("%gM", *size),
			"-vf", vfopt,
			"-c:v", "libx264",
			"-preset", *preset,
			"-b:v", fmt.Sprintf("%dk", vbitrate),
			"-pass", "2",
			"-passlogfile", file,
			"-movflags", "+faststart",
			"-c:a", "copy",
			"-an",
			output,
		)
	} else {
		pass2 = exec.Command(
			"ffmpeg", "-y",
			"-i", file,
			"-i", file+".m4a",
			//"-fs", fmt.Sprintf("%gM", *size),
			"-vf", vfopt,
			"-c:v", "libx264",
			"-preset", *preset,
			"-b:v", fmt.Sprintf("%dk", vbitrate),
			"-pass", "2",
			"-passlogfile", file,
			"-movflags", "+faststart",
			"-c:a", "copy",
			"-map", "0:v:0",
			"-map", "1:a:0",
			output,
		)
	}
	pass2.Stderr = os.Stderr
	pass2.Stdout = os.Stdout
	err = pass2.Run()
	if err != nil {
		cleanup()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// remove the mess
	cleanup()
}
