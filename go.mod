module spark

go 1.22.1

require (
	github.com/hajimehoshi/ebiten/v2 v2.7.10
	tinygo.org/x/drivers v0.33.0
	tinygo.org/x/tinyfont v0.6.0
	tinygo.org/x/tinyterm v0.1.0
)

replace tinygo.org/x/tinyterm => ./app/tinyterm-release

require (
	github.com/ebitengine/gomobile v0.0.0-20240518074828-e86332849895 // indirect
	github.com/ebitengine/hideconsole v1.0.0 // indirect
	github.com/ebitengine/purego v0.7.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/jezek/xgb v1.1.1 // indirect
	golang.org/x/sync v0.7.0 // indirect
	golang.org/x/sys v0.20.0 // indirect
)
