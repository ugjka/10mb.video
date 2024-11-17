# 10mb.video
Fit a video into a 10mb file (Discord nitro pls?)

## deps
Needs ffmpeg ffprobe fdkaac

Tested only on Linux/Termux

## building
To build this you need the Go compiler:

go build -o 10mb.video main.go


## caveats
ffmpeg doesn't like hi-res square still image videos (youtube music stuff),

the encoder will overshoot the bitrate by a huge margin in such case

to work around you may try "-down 512"

## usage
```
[ugjka@ugjka 10mb.video]$ 10mb.video -h
Usage: 10mb.video [OPTIONS] [FILE]

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
```
