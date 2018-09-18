<img src="https://cdn.rawgit.com/BigJk/7e61395616df18c9b6003aa90c77e829/raw/ec7bc03e02015deb0c96c6914f5c0460af773b59/ramen.svg" width="145" align="left" />

<img src="https://i.imgur.com/glpKbxk.png" width="10" height="145" align="left" />

[![Documentation](https://godoc.org/github.com/BigJk/ramen?status.svg)](http://godoc.org/github.com/BigJk/ramen) [![Go Report Card](https://goreportcard.com/badge/github.com/BigJk/ramen)](https://goreportcard.com/report/github.com/BigJk/ramen) [![Hex.pm](https://img.shields.io/hexpm/l/plug.svg)](LICENSE)

**ramen** is a simple console emulator written in go that can be used to create various ascii / text (roguelike) games. It's based on the great **[ebiten](https://github.com/hajimehoshi/ebiten)** library and inspired by libraries like **[libtcod](https://bitbucket.org/libtcod/libtcod/wiki/Features)**.

**Warning:** This is still a early version so api and features are not fixed yet

<br>

## Features

- PNG Fonts with more than 256 chars possible
- Fonts can contain chars and colored tiles
- Create sub-consoles to organize rendering
- Everything **ebiten** can do
  - Input: Mouse, Keyboard, Gamepads, Touches
  - Audio: MP3, Ogg/Vorbis, WAV, PCM
  - ...

## Get Started

```
go get github.com/BigJk/ramen
```

## Example

```go
package main

import (
	"github.com/BigJk/ramen/console"
	"github.com/BigJk/ramen/consolecolor"
	"github.com/BigJk/ramen/font"
	"github.com/BigJk/ramen/t"
	"github.com/hajimehoshi/ebiten"
)

func main() {
	// load a font you like
	font, err := font.New("../../fonts/terminus-11x11.png", 11, 11)
	if err != nil {
		panic(err)
	}

	// create a 50x30 cells console with the title 'ramen example'
	con, err := console.New(50, 30, font, "ramen example")
	if err != nil {
		panic(err)
	}

	// set a pre-render hook. This function will be executed
	// each frame before the drawing happened. You can also
	// call the console functions from other goroutines.
	// The timeDelta parameter is the elapsed time in seconds
	// since the last frame.
	con.SetPreRenderHook(func(screen *ebiten.Image, timeDelta float64) error {
		con.ClearAll(t.Background(consolecolor.New(50, 50, 50)))

		con.Print(2, 2, "Hello!", t.Foreground(consolecolor.New(0, 255, 0)), t.Background(consolecolor.New(255, 0, 0)))

		return nil
	})

	// start the console with a scaling of 1
	con.Start(1)
}

```

## Screenshots

<img src="./_resources/screen_colored_tiles.png" width="538" align="left">
<img src="./_resources/screen_text.png" width="200" align="left">
