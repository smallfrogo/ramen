// Package console provides a emulated console view.
package console

import (
	"fmt"
	"math"
	"strings"
	"sync"
  "regexp"
	"sort"

	"github.com/BigJk/ramen"
	"github.com/BigJk/ramen/concolor"
	"github.com/BigJk/ramen/font"
	"github.com/BigJk/ramen/t"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

var emptyCell = ramen.Cell{
	Foreground: concolor.RGB(255, 255, 255),
}

// Console represents a emulated console view.
type Console struct {
	Title       string
	Width       int
	Height      int
	Font        *font.Font
	ShowFPS     bool
	SubConsoles []*Console

	parent       *Console
	x            int
	y            int
	priority     int
	isSubConsole bool

	mtx    sync.RWMutex
	buffer [][]ramen.Cell

	mouseX int
	mouseY int

	components map[string]Component

	tickHook       func(timeElapsed float64) error
	preRenderHook  func(screen *ebiten.Image, timeElapsed float64) error
	postRenderHook func(screen *ebiten.Image, timeElapsed float64) error
}

// New creates a new console.
func New(width, height int, font *font.Font, title string) (*Console, error) {
	buf := make([][]ramen.Cell, width)
	for x := range buf {
		buf[x] = make([]ramen.Cell, height)
		for y := range buf[x] {
			buf[x][y] = emptyCell
		}
	}

	lines := make([]*ebiten.Image, width)
	for i := range lines {
		lines[i] = ebiten.NewImage(font.TileWidth, height*font.TileHeight)
	}

	return &Console{
		Title:       title,
		Width:       width,
		Height:      height,
		Font:        font,
		SubConsoles: make([]*Console, 0),
		buffer:      buf,
		components:  map[string]Component{},
	}, nil
}

// Update proceeds the game state and is called every tick (1/60 [s] by default).
// This is an ebiten function. Don't call it yourself!
func (c *Console) Update() error {
	c.mtx.RLock()
	mx, my := ebiten.CursorPosition()
	c.propagateMousePosition(mx/c.Font.TileWidth, my/c.Font.TileHeight)
	c.propagateComponentUpdates(c.elapsedTPS())
	c.mtx.RUnlock()

	if c.tickHook != nil {
		if err := c.tickHook(c.elapsedTPS()); err != nil {
			return err
		}
	}

	return nil
}

// Draw draws the game screen and is called every frame (typically 1/60[s] for 60Hz display).
// This is an ebiten function. Don't call it yourself!
func (c *Console) Draw(screen *ebiten.Image) {
	_ = c.update(screen)
}

// Layout returns size of drawable area inside window. This will be stretched to window sized.
// This is an ebiten function. Don't call it yourself!
func (c *Console) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return c.Width * c.Font.TileWidth, c.Height * c.Font.TileHeight
}

// Start will open the console window with the given scale.
func (c *Console) Start(scale float64) error {
	fmt.Println("starting")
	if c.isSubConsole {
		return fmt.Errorf("only the main console can be started")
	}

	ebiten.SetWindowSize(int(float64(c.Width*c.Font.TileWidth)*scale), int(float64(c.Height*c.Font.TileHeight)*scale))
	ebiten.SetWindowTitle(c.Title)
	return ebiten.RunGame(c)
}

// SetTickHook will apply a hook that gets triggered every tick, even if drawing is skipped in this tick.
// This is a good place for game logic as it runs disconnected from the fps.
func (c *Console) SetTickHook(hook func(timeElapsed float64) error) error {
	if c.isSubConsole {
		return fmt.Errorf("can't hook into sub-console")
	}
	c.tickHook = hook
	return nil
}

// SetPreRenderHook will apply a hook that gets triggered before the console started rendering.
// This is a good place to change the console or to draw extra content under the console.
func (c *Console) SetPreRenderHook(hook func(screen *ebiten.Image, timeElapsed float64) error) error {
	if c.isSubConsole {
		return fmt.Errorf("can't hook into sub-console")
	}
	c.preRenderHook = hook
	return nil
}

// SetPostRenderHook will apply a hook that gets triggered after the console is finished rendering.
// This is a good place if you want to draw some extra content over the console.
func (c *Console) SetPostRenderHook(hook func(screen *ebiten.Image, timeElapsed float64) error) error {
	if c.isSubConsole {
		return fmt.Errorf("can't hook into sub-console")
	}
	c.postRenderHook = hook
	return nil
}

// SetPriority sets the priority of the console. A higher priority will result in the console
// being drawn on top of all the ones with lower priority.
func (c *Console) SetPriority(priority int) error {
	if !c.isSubConsole {
		return fmt.Errorf("priority of the main console can't be changed")
	}
	c.priority = priority
	c.parent.sortSubConsoles()
	return nil
}

// AddComponent adds a component that should be updated and rendered to the console.
//
// Attention: Don't use this function inside a callback from a component as this will
// create a deadlock!
func (c *Console) AddComponent(component Component) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.components[component.ID()] = component
}

// RemoveComponent removes a component from the console.
//
// Attention: Don't use this function inside a callback from a component as this will
// create a deadlock!
func (c *Console) RemoveComponent(component Component) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	delete(c.components, component.ID())
}

// HasComponent checks if component is mounted to the console.
//
// Attention: Don't use this function inside a callback from a component as this will
// create a deadlock!
func (c *Console) HasComponent(component Component) bool {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	_, ok := c.components[component.ID()]
	return ok
}

// CreateSubConsole creates a new sub-console.
func (c *Console) CreateSubConsole(x, y, width, height int) (*Console, error) {
	if x < 0 || y < 0 || x+width > c.Width || y+height > c.Height || width <= 0 || height <= 0 {
		return nil, fmt.Errorf("sub-console is out of bounds")
	}

	c.mtx.Lock()

	sub, err := New(width, height, c.Font, "")
	if err != nil {
		return nil, err
	}

	sub.parent = c
	sub.x = x
	sub.y = y
	sub.isSubConsole = true

	c.SubConsoles = append(c.SubConsoles, sub)

	c.mtx.Unlock()

	c.sortSubConsoles()

	return sub, nil
}

// RemoveSubConsole removes a sub-console from his parent.
func (c *Console) RemoveSubConsole(con *Console) error {
	c.mtx.Lock()
	for i := range c.SubConsoles {
		if c.SubConsoles[i] == con {
			c.SubConsoles[i] = c.SubConsoles[len(c.SubConsoles)-1]
			c.SubConsoles[len(c.SubConsoles)-1] = nil
			c.SubConsoles = c.SubConsoles[:len(c.SubConsoles)-1]
			c.mtx.Unlock()

			c.sortSubConsoles()

			return nil
		}
	}
	c.mtx.Unlock()
	return fmt.Errorf("sub-console not found")
}

// ClearAll clears the whole console.
func (c *Console) ClearAll() error {
	return c.TransformAll(t.Cell(emptyCell))
}

// Clear clears part of the console.
func (c *Console) Clear(x, y, width, height int) error {
	return c.TransformArea(x, y, width, height, t.Cell(emptyCell))
}

// TransformAll applies the given transformers to all cells in the console.
func (c *Console) TransformAll(transformer ...t.Transformer) error {
	return c.TransformArea(0, 0, c.Width, c.Height, transformer...)
}

// TransformArea applies the given transformers to all cells in the given area.
func (c *Console) TransformArea(x, y, width, height int, transformer ...t.Transformer) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	for px := 0; px < width; px++ {
		for py := 0; py < height; py++ {
			if err := c.checkOutOfBounds(px+x, py+y); err != nil {
				return err
			}

			if len(transformer) == 0 {
				if c.buffer[px+x][py+y] != emptyCell {
					c.buffer[px+x][py+y] = emptyCell
				}
			} else {
				for i := range transformer {
					err := transformer[i].Transform(&c.buffer[px+x][py+y])
					if err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

// Transform transforms a cell. This can be used to change the character, foreground and
// background of a cell or apply custom transformers onto a cell.
func (c *Console) Transform(x, y int, transformer ...t.Transformer) error {
	if len(transformer) == 0 {
		return fmt.Errorf("no transformer given")
	} else if err := c.checkOutOfBounds(x, y); err != nil {
		return err
	}

	c.mtx.Lock()
	defer c.mtx.Unlock()

	for i := range transformer {
		err := transformer[i].Transform(&c.buffer[x][y])
		if err != nil {
			return err
		}
	}

	return nil
}

// PrintCtrAdj prints text onto the console but automatically centre adjusted to the length of the text.
func (c *Console) PrintCtrAdj(x, y int, text string, transformer ...t.Transformer) {
  var colorSectionRegex = regexp.MustCompile("\\[\\[(([bf]):(#[0-9a-zA-Z]+))(\\|(([bf]):(#[0-9a-zA-Z]+)))?\\]\\]")
	split := strings.Split(text,"\n")
  matches := colorSectionRegex.FindAllStringIndex(split[0], -1)
	x = x - (len(split[0])/2)
  x = x + (len(matches)*7)
	c.PrintBounded(x, y, 0, 0, text, transformer...)
}

// Print prints a text onto the console. To give the text a different foreground or
// background color use transformer. This function also supports inlined color
// definitions.
func (c *Console) Print(x, y int, text string, transformer ...t.Transformer) {
	c.PrintBounded(x, y, 0, 0, text, transformer...)
}

// PrintBounded prints a text onto the console that is bounded by a width and height.
// If you set width or height to <= 0 this bound won't have a limit.
// To give the text a different foreground or background color use transformer.
// This function also supports inlined color definitions.
func (c *Console) PrintBounded(x, y, width, height int, text string, transformer ...t.Transformer) int {
	return c.PrintBoundedOffset(x, y, width, height, 0, text, transformer...)
}

// PrintBoundedOffset prints a text onto the console that is bounded by a width and height
// and skips the first sy lines.
// If you set width or height to <= 0 this bound won't have a limit.
// To give the text a different foreground or background color use transformer.
// This function also supports inlined color definitions.
func (c *Console) PrintBoundedOffset(x, y, width, height, sy int, text string, transformer ...t.Transformer) int {
	cleaned, colors := ParseColoredText(text)

	line := 0
	linePos := 0
	for i, val := range cleaned {
		if cleaned[i] == '\n' || width > 0 && linePos >= width {
			y++
			linePos = 0
			line++

			if cleaned[i] == '\n' {
				continue
			}
		}

		if x+linePos >= c.Width || height > 0 && line >= height {
			continue
		}

		if line >= sy {
			trans := transformer
			trans = append(trans, t.Char(int(val)))
			trans = append(trans, colors.GetCurrentTransformer(i)...)

			c.Transform(linePos+x, y-sy, trans...)
		}

		linePos++
	}

	return line + 1 - sy
}

// CalcTextHeight pre-calculates the height a text will need.
func (c *Console) CalcTextHeight(width, height int, text string) int {
	cleaned, _ := ParseColoredText(text)

	line := 0
	linePos := 0
	for i := range cleaned {
		if cleaned[i] == '\n' || width > 0 && linePos >= width {
			linePos = 0
			line++

			if cleaned[i] == '\n' {
				continue
			}
		}

		if height > 0 && line >= height {
			continue
		}

		linePos++
	}

	return line + 1
}

// MousePosition returns the cell that the mouse cursor is currently in. If it returns
// (-1, -1) the mouse cursor is currently not in the console.
func (c *Console) MousePosition() (int, int) {
	return c.mouseX, c.mouseY
}

// MouseInArea checks if the mouse cursor is currently in the given area.
func (c *Console) MouseInArea(x, y, width, height int) bool {
	return c.mouseX >= x && c.mouseY >= y && c.mouseX < x+width && c.mouseY < y+height
}

func (c *Console) sortSubConsoles() {
	c.mtx.Lock()
	sort.SliceStable(c.SubConsoles, func(i, j int) bool {
		return c.SubConsoles[i].priority < c.SubConsoles[j].priority
	})
	c.mtx.Unlock()
}

func (c *Console) checkOutOfBounds(x, y int) error {
	if x < 0 || y < 0 || x >= c.Width || y >= c.Height {
		return fmt.Errorf("position out of bounds")
	}
	return nil
}

func (c *Console) draw(screen *ebiten.Image, timeElapsed float64, offsetX, offsetY int) {
	for id := range c.components {
		if c.components[id].ShouldDraw() {
			c.components[id].Draw(c, timeElapsed)
		}
	}

	c.mtx.RLock()
	for x := range c.buffer {
		for y := range c.buffer[x] {
			if c.buffer[x][y].Background.A == 0 {
				continue
			}

			ebitenutil.DrawRect(screen, float64((offsetX+c.x+x)*c.Font.TileWidth), float64((offsetY+c.y+y)*c.Font.TileHeight), float64(c.Font.TileWidth), float64(c.Font.TileHeight), c.buffer[x][y].Background)
		}
	}

	for x := range c.buffer {
		for y := range c.buffer[x] {
			charImage := c.Font.ToSubImage(c.buffer[x][y].Char)
			if charImage != nil {
				op := ebiten.DrawImageOptions{}
				if !c.Font.IsTile(c.buffer[x][y].Char) {
					op.ColorM.Scale(c.buffer[x][y].Foreground.Floats())
				}
				op.GeoM.Translate(float64((offsetX+c.x+x)*c.Font.TileWidth), float64((offsetY+c.y+y)*c.Font.TileHeight))
				screen.DrawImage(charImage, &op)
			}
		}
	}
	c.mtx.RUnlock()

	for i := range c.SubConsoles {
		c.SubConsoles[i].draw(screen, timeElapsed, offsetX+c.x, offsetY+c.y)
	}
}

func (c *Console) propagateMousePosition(x, y int) {
	c.mouseX = x - c.x
	c.mouseY = y - c.y

	if c.mouseX >= c.Width || c.mouseY >= c.Height {
		c.mouseX = -1
		c.mouseY = -1
	} else {
		for i := range c.SubConsoles {
			c.SubConsoles[i].propagateMousePosition(c.mouseX, c.mouseY)
		}
	}
}

func (c *Console) propagateComponentUpdates(timeElapsed float64) {
	for id := range c.components {
		if !c.components[id].ShouldDraw() {
			c.components[id].SetFocus(false)
		} else if c.components[id].FocusOnClick() && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			x, y := c.components[id].Position()
			w, h := c.components[id].Size()
			c.components[id].SetFocus(c.MouseInArea(x, y, w, h))
		}

		if c.components[id].ShouldClose() || !c.components[id].Update(c, timeElapsed) {
			delete(c.components, id)
		}
	}

	for id := range c.SubConsoles {
		c.SubConsoles[id].propagateComponentUpdates(timeElapsed)
	}
}

func (c *Console) elapsedTPS() float64 {
	e := 1.0 / math.Min(float64(ebiten.MaxTPS()), ebiten.CurrentTPS())
	if e > math.MaxFloat64 {
		e = 0
	}
	return e
}

func (c *Console) elapsedFPS() float64 {
	e := 1.0 / ebiten.CurrentFPS()
	if e > math.MaxFloat64 {
		e = 0
	}
	return e
}

func (c *Console) update(screen *ebiten.Image) error {
	timeElapsed := c.elapsedFPS()

	if c.preRenderHook != nil {
		if err := c.preRenderHook(screen, timeElapsed); err != nil {
			return err
		}
	}

	c.draw(screen, timeElapsed, 0, 0)

	if c.postRenderHook != nil {
		if err := c.postRenderHook(screen, timeElapsed); err != nil {
			return err
		}
	}

	if c.ShowFPS {
		ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %0.2f", ebiten.CurrentFPS()))
	}

	return nil
}
