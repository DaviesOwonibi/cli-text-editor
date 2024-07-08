package main

import (
	"bufio"
	"log"
	"os"

	"github.com/nsf/termbox-go"
)

type Selection struct {
	active         bool
	startX, startY int
	endX, endY     int
}

type CopiedText struct {
	active bool
	text   []rune
}

var (
	textBuffer       [][]rune
	cursorX, cursorY int
	selection        Selection
	copiedText       CopiedText
	filename         string
)

func readFileIntoBuffer(filename string) error {
	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		// If the file does not exist, create it
		file, err = os.Create(filename)
		if err != nil {
			return err
		}
	}
	defer file.Close()

	// Check if the file is empty
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	if stat.Size() == 0 {
		// If the file is empty, initialize textBuffer as an empty 2D array
		textBuffer = [][]rune{}
	} else {
		// Read lines from the file
		scanner := bufio.NewScanner(file)
		textBuffer = nil // Clear existing buffer
		for scanner.Scan() {
			textBuffer = append(textBuffer, []rune(scanner.Text()))
		}

		if err := scanner.Err(); err != nil {
			return err
		}
	}

	// Reset cursor position
	cursorX, cursorY = 0, 0
	return nil
}
func main() {
	filename = os.Args[1]
	initializeBuffer()
	err := readFileIntoBuffer(filename)
	if err != nil {
		log.Fatal(err)
	}

	err = termbox.Init()
	if err != nil {
		log.Fatal(err)
	}
	defer termbox.Close()

	drawUI()
	eventLoop()
}

func initializeBuffer() {
	textBuffer = [][]rune{
		[]rune("Start typing."),
	}
	cursorX, cursorY = 0, 0
	selection = Selection{}
	copiedText = CopiedText{}
}

func isSelected(x, y int) bool {
	if !selection.active {
		return false
	}
	startX, startY := selection.startX, selection.startY
	endX, endY := selection.endX, selection.endY
	if startY > endY || (startY == endY && startX > endX) {
		startX, startY, endX, endY = endX, endY, startX, startY
	}
	if y < startY || y > endY {
		return false
	}
	if y == startY && y == endY {
		return x >= startX && x <= endX
	}
	if y == startY {
		return x >= startX
	}
	if y == endY {
		return x <= endX
	}
	return true
}

func drawUI() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	for y, line := range textBuffer {
		for x, ch := range line {
			bg := termbox.ColorDefault
			if selection.active && isSelected(x, y) {
				bg = termbox.ColorBlue
			}
			termbox.SetCell(x, y, ch, termbox.ColorDefault, bg)
		}
	}
	termbox.SetCursor(cursorX, cursorY)
	termbox.Flush()
}

func eventLoop() {
	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			if ev.Key == termbox.KeyEsc {
				return
			}
			handleKeyEvent(ev)

		case termbox.EventError:
			log.Fatal(ev.Err)
		}
	}
}

func moveCursor(dy, dx int, selectMode bool) {
	if selectMode && !selection.active {
		selection.active = true
		selection.startX, selection.startY = cursorX, cursorY
	}

	if cursorY+dy >= 0 && cursorY+dy < len(textBuffer) {
		cursorY += dy
		if cursorX+dx >= 0 && cursorX+dx <= len(textBuffer[cursorY]) {
			cursorX += dx
		}
	}

	if selectMode {
		selection.endX, selection.endY = cursorX, cursorY
	} else {
		selection.active = false
	}
}

func handleKeyEvent(ev termbox.Event) {
	switch ev.Key {
	case termbox.KeyArrowUp:
		if selection.active {
			// Move cursor to the start of the selection or to the previous line if at the start of selection
			if cursorY > selection.startY ||
				(cursorY == selection.startY && cursorX > selection.startX) {
				moveCursor(selection.startY-cursorY, selection.startX-cursorX, false)
			} else {
				moveCursor(-1, 0, false)
			}
		} else {
			moveCursor(-1, 0, false)
		}
	case termbox.KeyArrowDown:
		moveCursor(1, 0, false)
	case termbox.KeyArrowLeft:
		if selection.active {
			if cursorX > selection.startX ||
				(cursorX == selection.startX && cursorY > selection.startY) {
				moveCursor(selection.startX-cursorX, selection.startY-cursorY, false)
			} else {
				moveCursor(0, -1, false)
			}
		} else {
			moveCursor(0, -1, false)
		}
	case termbox.KeyArrowRight:
		moveCursor(0, 1, false)
	case termbox.KeyBackspace, termbox.KeyBackspace2:
		if selection.active {
			deleteSelectedText()
		} else if cursorX > 0 {
			deleteCharacterBeforeCursor()
		} else if cursorY > 0 {
			mergeLines()
		}

	case termbox.KeyDelete:
		deleteCharacterAfterCursor()

	case termbox.KeyEnter:
		insertNewLine()

	case termbox.KeySpace:
		insertSpace()

	case termbox.KeyCtrlA:
		selectAll()
	case termbox.KeyCtrlC:
		copyText()
	case termbox.KeyCtrlQ:
		pasteText()
	case termbox.KeyCtrlS:
		saveBufferToFile()

	default:
		if ev.Ch != 0 {
			insertCharacter(ev.Ch)
		}
	}
	drawUI()
}

func deleteSelectedText() {
	startX, startY := selection.startX, selection.startY
	endX, endY := selection.endX, selection.endY

	// Ensure startY <= endY and startX <= endX
	if startY > endY || (startY == endY && startX > endX) {
		startX, startY, endX, endY = endX, endY, startX, startY
	}

	// Delete selected text line by line
	for y := startY; y <= endY; y++ {
		line := textBuffer[y]
		if y == startY {
			textBuffer[y] = append(line[:startX], line[endX:]...)
		} else if y == endY {
			textBuffer[y-1] = append(textBuffer[y-1], line[endX:]...)
			textBuffer = append(textBuffer[:y], textBuffer[y+1:]...)
		} else {
			textBuffer = append(textBuffer[:y], textBuffer[y+1:]...)
		}
	}

	// Update cursor position
	cursorX, cursorY = startX, startY
	selection.active = false
}

func deleteCharacterBeforeCursor() {
	if cursorY >= 0 && cursorX > 0 {
		line := textBuffer[cursorY]
		textBuffer[cursorY] = append(line[:cursorX-1], line[cursorX:]...)
		cursorX--
	}
}

func mergeLines() {
	if cursorY > 0 {
		previousLine := textBuffer[cursorY-1]
		currentLine := textBuffer[cursorY]
		cursorX = len(previousLine)
		textBuffer[cursorY-1] = append(previousLine, currentLine...)
		textBuffer = append(textBuffer[:cursorY], textBuffer[cursorY+1:]...)
		cursorY--
	}
}

func deleteCharacterAfterCursor() {
	if cursorY < len(textBuffer) {
		line := textBuffer[cursorY]
		if cursorX < len(line) {
			textBuffer[cursorY] = append(line[:cursorX], line[cursorX+1:]...)
		}
	}
}

func insertNewLine() {
	line := textBuffer[cursorY]
	newLine := line[cursorX:]
	textBuffer[cursorY] = line[:cursorX]
	textBuffer = append(
		textBuffer[:cursorY+1],
		append([][]rune{newLine}, textBuffer[cursorY+1:]...)...)
	cursorX = 0
	cursorY++
}

func insertSpace() {
	if textBuffer == nil || cursorY < 0 || cursorY >= len(textBuffer) {
		textBuffer = [][]rune{{}}
		cursorY = 0
	}

	if cursorX < 0 {
		cursorX = 0
	} else if cursorX > len(textBuffer[cursorY]) {
		cursorX = len(textBuffer[cursorY])
	}

	line := textBuffer[cursorY]
	textBuffer[cursorY] = append(line[:cursorX], append([]rune{' '}, line[cursorX:]...)...)
	cursorX++
}

func insertCharacter(ch rune) {
	if textBuffer == nil || cursorY < 0 || cursorY >= len(textBuffer) {
		// Initialize textBuffer with an empty line if it's nil or cursorY is out of range
		textBuffer = [][]rune{{}}
		cursorY = 0
	}

	// Ensure cursorX is within valid range for the current line
	if cursorX < 0 {
		cursorX = 0
	} else if cursorX > len(textBuffer[cursorY]) {
		cursorX = len(textBuffer[cursorY])
	}

	line := textBuffer[cursorY]
	textBuffer[cursorY] = append(line[:cursorX], append([]rune{ch}, line[cursorX:]...)...)
	cursorX++
}

func selectAll() {
	selection.active = true
	selection.startX, selection.startY = 0, 0
	selection.endY = len(textBuffer) - 1
	selection.endX = len(textBuffer[selection.endY])
	cursorX = selection.endX
	cursorY = selection.endY
}

func copyText() {
	if !selection.active {
		return
	}

	startX, startY := selection.startX, selection.startY
	endX, endY := selection.endX, selection.endY

	// Ensure startY <= endY and startX <= endX
	if startY > endY || (startY == endY && startX > endX) {
		startX, startY, endX, endY = endX, endY, startX, startY
	}

	// Copy selected text to clipboard or store it somewhere
	copiedText.text = nil // Clear previous copied text
	for y := startY; y <= endY; y++ {
		line := textBuffer[y]
		if y == startY && y == endY {
			copiedText.text = append(copiedText.text, line[startX:endX]...)
		} else if y == startY {
			copiedText.text = append(copiedText.text, line[startX:]...)
		} else if y == endY {
			copiedText.text = append(copiedText.text, line[:endX]...)
		} else {
			copiedText.text = append(copiedText.text, line...)
		}
	}
	copiedText.active = true
}

func pasteText() {
	if !copiedText.active || len(copiedText.text) == 0 {
		return
	}

	// Insert copied text into buffer at cursor position
	for _, ch := range copiedText.text {
		// Ensure cursorY is within valid range
		if cursorY < 0 {
			cursorY = 0
		} else if cursorY > len(textBuffer) {
			cursorY = len(textBuffer)
		}

		// Insert the character at cursorX position
		if cursorY < len(textBuffer) {
			textBuffer[cursorY] = append(
				textBuffer[cursorY],
				' ',
			) // Ensure enough space at end of line
			copy(textBuffer[cursorY][cursorX+1:], textBuffer[cursorY][cursorX:])
			textBuffer[cursorY][cursorX] = ch
		} else {
			// Append new line if cursorY is out of range
			textBuffer = append(textBuffer, []rune{ch})
		}
		cursorX++
	}

	// Move cursor to the end of pasted text
	cursorY = len(textBuffer) - 1
	cursorX = len(textBuffer[cursorY])
}
func saveBufferToFile() error {
	// Open the file for writing
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write each line of textBuffer to the file
	for _, line := range textBuffer {
		if _, err := file.WriteString(string(line) + "\n"); err != nil {
			return err
		}
	}

	return nil
}
