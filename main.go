package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/mattn/go-runewidth"
	"github.com/nsf/termbox-go"
)

var (
	ROWS, COLS             int
	offsetRow, offsetCol   int
	currentCol, currentRow int
	source_file            string
	source_file2           string
	mode                   int
	text_buffer            [][]rune = [][]rune{}
	undoStack              []EditorState
	redoStack              []EditorState
	copy_buffer            []rune = []rune{}
	modified               int
	searchHighlights       []struct{ row, startCol, endCol int }
	searchQuery            string
	file_extension         string
	parentDir              string
	homeDir                string
	bytesWritten           int = 0
	selectionStart         struct{ row, col int }
	selectionEnd           struct{ row, col int }
	lineNumberWidth        int = 5
)

type EditorState struct {
	buffer    [][]rune
	cursorRow int
	cursorCol int
	offsetRow int
	offsetCol int
}

const (
	maxUndoLevels int = 500
	tabWidth      int = 1
)

func findText() {
	searchHighlights = []struct{ row, startCol, endCol int }{}
	searchQuery = ""
	mode = 2
	highlightIndex := 0

	for {
		termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
		display_text_buffer()
		print_message(0, ROWS, termbox.ColorWhite, termbox.ColorDefault, " "+string('\ue23e')+"  "+string('\uf002')+" SEARCH: "+searchQuery+" ")
		termbox.SetCursor(len("  SEARCH: ")+len(searchQuery)+4, ROWS)
		termbox.Flush()

		ev := termbox.PollEvent()
		switch ev.Type {
		case termbox.EventKey:
			switch ev.Key {
			case termbox.KeyEsc:
				mode = 0
				if len(searchHighlights) > 0 && searchQuery != "" {
					currentCol = searchHighlights[highlightIndex].startCol
				}
				searchHighlights = []struct{ row, startCol, endCol int }{}
				return
			case termbox.KeyEnter:
				if len(searchHighlights) > 0 && searchQuery != "" {
					if highlightIndex >= len(searchHighlights) {
						highlightIndex = 0 // Loop back to the start if at the end
					}
					currentRow = searchHighlights[highlightIndex].row
					currentCol = searchHighlights[highlightIndex].startCol
					lineToJump := currentRow + 1
					jumpToLine(&lineToJump)
					highlightIndex++
				}
			case termbox.KeyBackspace, termbox.KeyBackspace2:
				if len(searchQuery) > 0 {
					searchQuery = searchQuery[:len(searchQuery)-1]
				}
			default:
				if ev.Ch != 0 {
					searchQuery += string(ev.Ch)
				} else if ev.Key == termbox.KeySpace {
					searchQuery += " "
				}
			}
		}

		if searchQuery != "" {
			searchHighlights = []struct{ row, startCol, endCol int }{}
			lowerSearchQuery := strings.ToLower(searchQuery)

			for i, line := range text_buffer {
				lineStr := string(line)
				lowerLineStr := strings.ToLower(lineStr)
				index := 0
				for {
					startIndex := strings.Index(lowerLineStr[index:], lowerSearchQuery)
					if startIndex == -1 {
						break
					}
					startIndex += index
					endIndex := startIndex + len(lowerSearchQuery)
					searchHighlights = append(searchHighlights, struct{ row, startCol, endCol int }{i, startIndex, endIndex})
					index = endIndex
				}
			}

			// Check if any of the highlights are within the current view
			inView := false
			for _, highlight := range searchHighlights {
				if highlight.row >= currentRow && highlight.row < currentRow+ROWS {
					inView = true
					break
				}
			}

			// If no highlights are in view, jump to the first result
			if !inView && len(searchHighlights) > 0 {
				highlightIndex = 0
				currentRow = searchHighlights[highlightIndex].row
				currentCol = searchHighlights[highlightIndex].startCol
				lineToJump := currentRow + 1
				jumpToLine(&lineToJump)
			}
		} else {
			searchHighlights = []struct{ row, startCol, endCol int }{} // Clear highlights if search query is empty
		}
	}
}

func ClearScreen() {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		cmd.Run()
	} else {
		cmd := exec.Command("clear")
		cmd.Stdout = os.Stdout
		cmd.Run()
	}
}

func read_file(filename string) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting current directory:", err)
		os.Exit(1)
	}

	fullPath := filepath.Join(cwd, filename)
	ext := filepath.Ext(filename)
	file_extension = strings.TrimPrefix(ext, ".")
	parentDir = filepath.Base(filepath.Dir(fullPath))
	dirname, err := os.UserHomeDir()
	if err != nil {
		fmt.Println(err)
	}
	homeDir = strings.Trim(dirname, ".")

	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			file, err = os.Create(filename)
			if err != nil {
				ClearScreen()
				fmt.Println("Error creating file:", err)
				os.Exit(1)
			}
		} else {
			ClearScreen()
			fmt.Println("Error:", err)
			os.Exit(1)
		}
	} else {
		defer file.Close()
	}

	scanner := bufio.NewScanner(file)
	lineNumber := 0

	for scanner.Scan() {
		line := scanner.Text()
		text_buffer = append(text_buffer, []rune{})

		for _, r := range line {
			unicodeStr := fmt.Sprintf("\\u{%X}", r)
			hexStr := strings.Trim(unicodeStr, "\\u{}")

			codePoint, err := strconv.ParseInt(hexStr, 16, 32)
			if err != nil {
				fmt.Println("Error parsing Unicode code point:", err)
				return
			}

			r = rune(codePoint)
			text_buffer[lineNumber] = append(text_buffer[lineNumber], r)
		}
		lineNumber++
	}

	if lineNumber == 0 {
		text_buffer = append(text_buffer, []rune{})
	}
}

func insert_rune(event termbox.Event) {
	push_buffer()
	insert_rune := make([]rune, len(text_buffer[currentRow])+1)
	copy(insert_rune[:currentCol], text_buffer[currentRow][:currentCol])
	if event.Key == termbox.KeySpace {
		insert_rune[currentCol] = rune(' ')
	} else if event.Key == termbox.KeyTab {
		for i := 0; i < tabWidth; i++ {
			insert_rune[currentCol] = rune(' ')
		}
	} else {
		insert_rune[currentCol] = rune(event.Ch)
	}
	copy(insert_rune[currentCol+1:], text_buffer[currentRow][currentCol:])
	text_buffer[currentRow] = insert_rune
	currentCol++
}

func delete_rune() {
	push_buffer()
	if currentCol > 0 {
		currentCol--
		delete_line := make([]rune, len(text_buffer[currentRow])-1)
		copy(delete_line[:currentCol], text_buffer[currentRow][:currentCol])
		copy(delete_line[currentCol:], text_buffer[currentRow][currentCol+1:])
		text_buffer[currentRow] = delete_line
	} else if currentRow > 0 {
		append_line := make([]rune, len(text_buffer[currentRow]))
		copy(append_line, text_buffer[currentRow][currentCol:])
		new_text_buffer := make([][]rune, len(text_buffer)-1)
		copy(new_text_buffer[:currentRow], text_buffer[:currentRow])
		copy(new_text_buffer[currentRow:], text_buffer[currentRow+1:])
		text_buffer = new_text_buffer
		currentRow--
		currentCol = len(text_buffer[currentRow])
		insert_line := make([]rune, len(text_buffer[currentRow])+len(append_line))
		copy(insert_line[:len(text_buffer[currentRow])], text_buffer[currentRow])
		copy(insert_line[len(text_buffer[currentRow]):], append_line)
		text_buffer[currentRow] = insert_line
	}
}

func delete_right_rune() {
	push_buffer()
	if currentCol < len(text_buffer[currentRow]) {
		// Delete the character at the current position
		delete_line := make([]rune, len(text_buffer[currentRow])-1)
		copy(delete_line[:currentCol], text_buffer[currentRow][:currentCol])
		copy(delete_line[currentCol:], text_buffer[currentRow][currentCol+1:])
		text_buffer[currentRow] = delete_line
	} else if currentRow < len(text_buffer)-1 {
		// If at the end of a line, join with the next line
		append_line := make([]rune, len(text_buffer[currentRow+1]))
		copy(append_line, text_buffer[currentRow+1])

		// Remove the next line from text_buffer
		new_text_buffer := make([][]rune, len(text_buffer)-1)
		copy(new_text_buffer[:currentRow+1], text_buffer[:currentRow+1])
		copy(new_text_buffer[currentRow+1:], text_buffer[currentRow+2:])
		text_buffer = new_text_buffer

		// Append the next line to the current line
		insert_line := make([]rune, len(text_buffer[currentRow])+len(append_line))
		copy(insert_line[:len(text_buffer[currentRow])], text_buffer[currentRow])
		copy(insert_line[len(text_buffer[currentRow]):], append_line)
		text_buffer[currentRow] = insert_line
	}
	// Note: The cursor position doesn't change when deleting to the right
}

func insert_line() {
	push_buffer()
	right_line := make([]rune, len(text_buffer[currentRow][currentCol:]))
	copy(right_line, text_buffer[currentRow][currentCol:])
	left_line := make([]rune, len(text_buffer[currentRow][:currentCol]))
	copy(left_line, text_buffer[currentRow][:currentCol])
	text_buffer[currentRow] = left_line
	currentRow++
	currentCol = 0
	new_text_buffer := make([][]rune, len(text_buffer)+1)
	copy(new_text_buffer, text_buffer[:currentRow])
	new_text_buffer[currentRow] = right_line
	copy(new_text_buffer[currentRow+1:], text_buffer[currentRow:])
	text_buffer = new_text_buffer
}

func copy_line() {
	copy_line := make([]rune, len(text_buffer[currentRow]))
	copy(copy_line, text_buffer[currentRow])
	copy_buffer = copy_line
	write_to_clipboard(copy_line)
}

func copy_selection() {
	if mode != 4 {
		return
	}

	// Clear the existing copy buffer
	copy_buffer = []rune{}

	// Determine the start and end points of the selection
	startRow, startCol := selectionStart.row, selectionStart.col
	endRow, endCol := selectionEnd.row, selectionEnd.col

	// Ensure start is before end
	if startRow > endRow || (startRow == endRow && startCol > endCol) {
		startRow, startCol, endRow, endCol = endRow, endCol, startRow, startCol
	}

	// Copy the selected text
	for row := startRow; row <= endRow; row++ {
		lineStart, lineEnd := 0, len(text_buffer[row])

		if row == startRow {
			lineStart = startCol
		}
		if row == endRow {
			lineEnd = endCol
		}

		copy_buffer = append(copy_buffer, text_buffer[row][lineStart:lineEnd]...)

		// Add a newline character if it's not the last line
		if row < endRow {
			copy_buffer = append(copy_buffer, '\n')
		}
	}

	// Write to clipboard
	write_to_clipboard(copy_buffer)
}

func paste_line() {
	push_buffer()
	content, err := clipboard.ReadAll()
	if err != nil {
		return
	}

	var pasteContent []rune
	if len(content) != 0 {
		pasteContent = []rune(content)
	} else if len(copy_buffer) != 0 {
		pasteContent = copy_buffer
	}

	if len(pasteContent) != 0 {
		new_text_buffer := make([][]rune, len(text_buffer)+1)
		copy(new_text_buffer[:currentRow], text_buffer[:currentRow])
		new_text_buffer[currentRow] = make([]rune, len(pasteContent))
		copy(new_text_buffer[currentRow], pasteContent)
		copy(new_text_buffer[currentRow+1:], text_buffer[currentRow:])
		text_buffer = new_text_buffer
		currentCol = 0
		currentRow++
	}
}

func paste_line_below() {
	push_buffer()
	content, err := clipboard.ReadAll()
	if err != nil {
		return
	}

	var pasteContent []rune
	if len(content) != 0 {
		pasteContent = []rune(content)
	} else if len(copy_buffer) != 0 {
		pasteContent = copy_buffer
	}

	if len(pasteContent) != 0 {
		if currentRow < len(text_buffer) {
			currentRow++
		} else {
			currentRow = len(text_buffer)
		}
		currentCol = 0
		new_text_buffer := make([][]rune, len(text_buffer)+1)
		copy(new_text_buffer[:currentRow], text_buffer[:currentRow])
		new_text_buffer[currentRow] = make([]rune, len(pasteContent))
		copy(new_text_buffer[currentRow], pasteContent)
		copy(new_text_buffer[currentRow+1:], text_buffer[currentRow:])
		text_buffer = new_text_buffer
	}
}

func cut_line() {
	push_buffer()
	copy_line()
	if currentRow >= len(text_buffer) || len(text_buffer) < 1 {
		return
	}
	if len(text_buffer) == 1 {
		text_buffer = [][]rune{}
	} else {
		new_text_buffer := make([][]rune, len(text_buffer)-1)
		copy(new_text_buffer[:currentRow], text_buffer[:currentRow])
		copy(new_text_buffer[currentRow:], text_buffer[currentRow+1:])
		text_buffer = new_text_buffer
	}
	if currentRow >= len(text_buffer) {
		currentRow = len(text_buffer) - 1
	}
	currentCol = min(currentCol, len(text_buffer[currentRow]))
}

func push_buffer() {
	state := EditorState{
		buffer:    make([][]rune, len(text_buffer)),
		cursorRow: currentRow,
		cursorCol: currentCol,
		offsetRow: offsetRow,
		offsetCol: offsetCol,
	}
	for i := range text_buffer {
		state.buffer[i] = make([]rune, len(text_buffer[i]))
		copy(state.buffer[i], text_buffer[i])
	}
	undoStack = append(undoStack, state)

	// Limit the undo stack size
	if len(undoStack) > maxUndoLevels {
		undoStack = undoStack[1:]
	}
}

func pull_buffer() {
	if len(undoStack) == 0 {
		modified = 1
		return
	}
	redoStack = append(redoStack, EditorState{
		buffer:    text_buffer,
		cursorRow: currentRow,
		cursorCol: currentCol,
		offsetRow: offsetRow,
		offsetCol: offsetCol,
	})

	// Restore the last state from the undo stack
	lastState := undoStack[len(undoStack)-1]
	text_buffer = lastState.buffer
	currentRow = lastState.cursorRow
	currentCol = lastState.cursorCol
	offsetRow = lastState.offsetRow
	offsetCol = lastState.offsetCol

	undoStack = undoStack[:len(undoStack)-1]
}

func jumpToLine(initialLineNumber *int) {
	mode = 3

	if initialLineNumber != nil {
		// Jump to the initial line directly
		lineNumber := *initialLineNumber
		if lineNumber > 0 && lineNumber <= len(text_buffer) {
			currentRow = lineNumber - 1
			currentCol = 0

			// Calculate the maximum offset that would display the last line at the bottom
			maxOffset := len(text_buffer) - ROWS
			if maxOffset < 0 {
				maxOffset = 0
			}

			// Set the offset to center the cursor, but don't exceed maxOffset
			offsetRow = currentRow - ROWS/2
			if offsetRow > maxOffset {
				offsetRow = maxOffset
			}
			if offsetRow < 0 {
				offsetRow = 0
			}
		}
		mode = 0
		return
	}

	// Handle interactive input if no initial line number is provided
	lineNumberStr := ""
	for {
		termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
		display_text_buffer()
		print_message(0, ROWS, termbox.ColorWhite, termbox.ColorDefault, " "+string('\ue23e')+" Jump to line: "+lineNumberStr)
		termbox.SetCursor(len("Jump to line: ")+len(lineNumberStr)+3, ROWS)
		termbox.Flush()

		ev := termbox.PollEvent()
		switch ev.Type {
		case termbox.EventKey:
			switch ev.Key {
			case termbox.KeyEsc:
				mode = 0
				return
			case termbox.KeyEnter:
				if lineNumber, err := strconv.Atoi(lineNumberStr); err == nil && lineNumber > 0 && lineNumber <= len(text_buffer) {
					currentRow = lineNumber - 1
					currentCol = 0

					// Calculate the maximum offset that would display the last line at the bottom
					maxOffset := len(text_buffer) - ROWS
					if maxOffset < 0 {
						maxOffset = 0
					}

					// Set the offset to center the cursor, but don't exceed maxOffset
					offsetRow = currentRow - ROWS/2
					if offsetRow > maxOffset {
						offsetRow = maxOffset
					}
					if offsetRow < 0 {
						offsetRow = 0
					}
				}
				mode = 0
				return
			case termbox.KeyBackspace, termbox.KeyBackspace2:
				if len(lineNumberStr) > 0 {
					lineNumberStr = lineNumberStr[:len(lineNumberStr)-1]
				}
			default:
				if ev.Ch >= '0' && ev.Ch <= '9' {
					lineNumberStr += string(ev.Ch)
				}
			}
		}
	}
}

func write_file(filename string) {
	file, err := os.Create(filename)
	if err != nil {
		fmt.Println("Error creating file:", err)
		return
	}
	defer file.Close()

	// Create a UTF-8 encoder
	writer := bufio.NewWriter(file)

	for _, line := range text_buffer {
		bytesWrited, err := writer.Write([]byte(string(line) + "\n"))
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return
		}
		bytesWritten += bytesWrited
	}

	err = writer.Flush()
	if err != nil {
		fmt.Println("Error closing writer:", err)
		return
	}

	modified = 2
}

func scroll_text_buffer() {
	if currentRow < offsetRow {
		offsetRow = currentRow
	}

	if currentCol < offsetCol {
		offsetCol = currentCol
	}

	if currentRow >= offsetRow+ROWS {
		offsetRow = currentRow - ROWS + 1
	}
	if currentCol >= offsetCol+COLS-lineNumberWidth {
		offsetCol = currentCol - COLS + lineNumberWidth + 1
	}
}

func display_text_buffer() {
	var row, col int

	for row = 0; row < ROWS; row++ {
		text_buffer_row := row + offsetRow

		// Display line number
		lineNumber := fmt.Sprintf("%*d", lineNumberWidth-1, text_buffer_row+1)
		lineColor := termbox.ColorBlack
		if currentRow == text_buffer_row {
			lineColor = termbox.ColorLightGray
		}
		for i, ch := range lineNumber {
			termbox.SetCell(i, row, ch, lineColor, termbox.ColorDefault)
		}
		termbox.SetCell(lineNumberWidth-1, row, '│', lineColor, termbox.ColorDefault)

		if text_buffer_row < len(text_buffer) {
			line := text_buffer[text_buffer_row]
			visibleCol := 0   // Track the visible column on the screen
			columnInLine := 0 // Track the current column in the line

			for col = 0; col < COLS-lineNumberWidth && visibleCol < COLS-lineNumberWidth; col++ {
				text_buffer_column := col + offsetCol

				if text_buffer_column < len(line) {
					ch := line[text_buffer_column]

					// Check if this character is part of a search highlight
					highlighted := false
					for _, highlight := range searchHighlights {
						if highlight.row == text_buffer_row &&
							text_buffer_column >= highlight.startCol &&
							text_buffer_column < highlight.endCol {
							highlighted = true
							break
						}
					}
					isSelected := mode == 4 && isWithinSelection(text_buffer_row, text_buffer_column)
					if ch == ' ' {
						bgColor := termbox.ColorDefault
						if highlighted {
							bgColor = termbox.ColorYellow
						}
						if isSelected {
							bgColor = termbox.ColorDarkGray
						}
						termbox.SetCell(visibleCol+lineNumberWidth, row, ' ', termbox.ColorDefault, bgColor)
					}
					if ch == '\t' {
						// Calculate the number of spaces needed for the tab
						spacesToAdd := tabWidth - (columnInLine % tabWidth)
						bgColor := termbox.ColorDefault
						if highlighted {
							bgColor = termbox.ColorYellow
						}
						if isSelected {
							bgColor = termbox.ColorDarkGray
						}
						for i := 0; i < spacesToAdd && visibleCol < COLS-lineNumberWidth; i++ {
							termbox.SetCell(visibleCol+lineNumberWidth, row, ' ', termbox.ColorDefault, bgColor)
							visibleCol++
						}
						columnInLine += spacesToAdd
					} else {
						fgColor := termbox.ColorDefault
						bgColor := termbox.ColorDefault
						if highlighted {
							fgColor = termbox.ColorBlack
							bgColor = termbox.ColorWhite
						}
						if isSelected {
							fgColor = termbox.ColorBlack
							bgColor = termbox.ColorDarkGray
						}
						termbox.SetCell(visibleCol+lineNumberWidth, row, ch, fgColor, bgColor)
						visibleCol++
						columnInLine++
					}
				}
			}
		} else if row+offsetRow > len(text_buffer)-1 {
			termbox.SetCell(lineNumberWidth, row, '~', termbox.ColorBlue, termbox.ColorDefault)
		}
	}
}

func isWithinSelection(row, col int) bool {
	if selectionStart.row <= selectionEnd.row {
		if row < selectionStart.row || row > selectionEnd.row {
			return false
		}
		if row == selectionStart.row && col < selectionStart.col {
			return false
		}
		if row == selectionEnd.row && col > selectionEnd.col {
			return false
		}
	} else {
		if row > selectionStart.row || row < selectionEnd.row {
			return false
		}
		if row == selectionEnd.row && col < selectionEnd.col {
			return false
		}
		if row == selectionStart.row && col > selectionStart.col {
			return false
		}
	}
	return true
}

func display_status_bar() {
	var mode_status string
	var file_status string
	var copy_status string
	var undo_status string
	var logo rune

	if mode == 1 {
		mode_status = " " + string('\ue23e') + "  INSERT "
	} else if mode == 2 {
		mode_status = " " + string('\ue23e') + " " + string('\uf002') + "  SEARCH: "
	} else if mode == 3 {
		mode_status = " " + string('\ue23e') + "  JUMP TO: "
	} else if mode == 4 {
		mode_status = " " + string('\ue23e') + "  VISUAL "
	} else {
		mode_status = " " + string('\ue23e') + "  NORMAL "
	}

	filename_length := len(source_file)
	switch file_extension {
	case "astro":
		logo = '\ue6b3'
	case "asm":
		logo = ''
	case "bat":
		logo = ''
	case "bash":
		logo = ''
	case "c":
		logo = ''
	case "cs":
		logo = ''
	case "cpp":
		logo = ''
	case "css":
		logo = ''
	case "csv":
		logo = ''
	case "cr":
		logo = ''
	case "cmake":
		logo = ''
	case "dart":
		logo = ''
	case "docker":
		logo = ''
	case "ex":
		logo = ''
	case "exs":
		logo = ''
	case "html":
		logo = ''
	case "hpp":
		logo = ''
	case "hs":
		logo = ''
	case "lhs":
		logo = ''
	case "go":
		logo = ''
	case "sum":
		logo = ''
	case "mod":
		logo = ''
	case "jsx":
		logo = ''
	case "kotlin":
		logo = ''
	case "tsx":
		logo = ''
	case "java":
		logo = ''
	case "lua":
		logo = ''
	case "md":
		logo = ''
	case "php":
		logo = '󰌟'
	case "ps1":
		logo = '󰨊'
	case "py":
		logo = ''
	case "vimrc":
		logo = ''
	case "vim":
		logo = ''
	case "js":
		logo = ''
	case "json":
		logo = ''
	case "rs":
		logo = ''
	case "rb":
		logo = ''
	case "sh":
		logo = ''
	case "gitignore":
		logo = ''
	case "sql":
		logo = ''
	case "sqlite":
		logo = ''
	case "db":
		logo = ''
	case "swift":
		logo = ''
	case "toml":
		logo = ''
	case "txt":
		logo = ''
	case "scss":
		logo = ''
	case "sass":
		logo = ''
	case "ts":
		logo = ''
	case "exe":
		logo = ''
	case "prisma":
		logo = ''
	case "tmux":
		logo = ''
	case "vue":
		logo = ''
	case "wasm":
		logo = ''
	case "yaml":
		logo = ''
	case "yml":
		logo = ''
	case "zsh":
		logo = ''
	default:
		logo = ''
	}

	if filename_length > 25 {
		filename_length = 25
	}

	filename_length = min(filename_length, len(source_file))

	if len(text_buffer) > 1 {
		file_status = string(logo) + " " + source_file2[:filename_length] + " " + strconv.Itoa(len(text_buffer)) + " lines"
	} else {
		file_status = string(logo) + " " + source_file2[:filename_length] + " " + strconv.Itoa(len(text_buffer)) + " line"
	}
	if modified == 0 {
		file_status += " modified"
	} else if modified == 1 {
		file_status += " oldest change"
	} else if modified == 2 {
		if bytesWritten == 0 {
			file_status += " saved 0 bytes"
		} else {
			file_status += " saved " + strconv.Itoa(bytesWritten) + " bytes"
			bytesWritten = 0
		}
	}
	var parent_status string
	homeDir = filepath.Base(homeDir)

	if parentDir == homeDir {
		parent_status = string('\uf015') + " " + parentDir + " "
	} else {
		parent_status = string('\uf07b') + " " + parentDir + " "
	}

	file_percent := string('\ue64e') + " " + strconv.Itoa((currentRow+1)*100/len(text_buffer)) + "%"
	if len(copy_buffer) > 0 {
		copy_status = " [Copy]"
	}
	if len(undoStack) > 0 {
		undo_status = " [Undo]"
	}
	used_space := len(mode_status) + len(file_status) + len(copy_status) + len(undo_status) + len(file_percent) + len(parent_status) + len("ROWS: "+strconv.Itoa(currentRow+1)+" COLS: "+strconv.Itoa(currentCol+1)) - 20
	spaces := strings.Repeat(" ", COLS-used_space)
	message := mode_status + file_status + copy_status + undo_status + spaces + parent_status + file_percent
	print_message(0, ROWS, termbox.ColorWhite, termbox.ColorDefault, message)
}

func print_message(column int, row int, fg termbox.Attribute, bg termbox.Attribute, message string) {
	for _, ch := range message {
		termbox.SetCell(column, row, ch, fg, bg)
		column += runewidth.RuneWidth(ch)
	}
}

func get_key() termbox.Event {
	var key_event termbox.Event
	switch event := termbox.PollEvent(); event.Type {
	case termbox.EventKey:
		key_event = event
	case termbox.EventError:
		panic(event.Err)
	}
	return key_event
}

func write_to_clipboard(runes []rune) {
	string_to_write := string(runes)
	err := clipboard.WriteAll(string_to_write)
	if err != nil {
		return
	}
}

func handle_close() {
	if modified != 0 {
		termbox.Close()
		os.Exit(0)
	} else {
		var answer string
		for {
			termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
			display_text_buffer()
			print_message(0, ROWS, termbox.ColorWhite, termbox.ColorDefault, " Would you like to save before leaving(y/n): "+answer)
			termbox.SetCursor(len("Would you like to save before leaving (y/n): ")+len(answer), ROWS)
			termbox.Flush()

			ev := termbox.PollEvent()
			if ev.Type == termbox.EventKey {
				switch ev.Key {
				case termbox.KeyEnter:
					if answer == "y" {
						write_file(source_file)
						termbox.Close()
						os.Exit(0)
					} else if answer == "n" {
						termbox.Close()
						os.Exit(0)
					}
				case termbox.KeyBackspace, termbox.KeyBackspace2:
					if len(answer) > 0 {
						answer = answer[:len(answer)-1]
					}
				default:
					if ev.Ch == 'y' || ev.Ch == 'n' {
						answer = string(ev.Ch)
					}
				}
			}
		}
	}
}

func process_key_press() {
	key_event := get_key()
	if key_event.Key == termbox.KeyEsc {
		mode = 0
	} else if key_event.Ch != 0 {
		if mode == 1 {
			insert_rune(key_event)
			modified = 0
		} else {
			switch key_event.Ch {
			case 'q':
				handle_close()
			case 'i':
				mode = 1
			case 'v':
				mode = 4
				selectionStart.row = currentRow
				selectionStart.col = currentCol
				selectionEnd.row = currentRow
				selectionEnd.col = currentCol
			case 'w':
				write_file(source_file)
			case 'y':
				if mode != 4 {
					copy_line()
				} else {
					copy_selection()
					mode = 0
				}
			case 'P':
				paste_line()
				modified = 0
			case 'd':
				if currentRow != 0 {
					cut_line()
					modified = 0
				} else {
				}
			case 'u':
				pull_buffer()
			case 'p':
				paste_line_below()
				modified = 0
			case '/':
				findText()
			case 'g':
				jumpToLine(nil)
			case 'k':
				if mode != 4 {
					if currentRow != 0 {
						currentRow--
					}
				} else {
					if currentRow != 0 {
						currentRow--
					}
					selectionEnd.row = currentRow
					selectionEnd.col = currentCol
				}

			case 'j':
				if mode != 4 {
					if currentRow < len(text_buffer)-1 {
						currentRow++
					}
				} else {
					if currentRow < len(text_buffer)-1 {
						currentRow++
					}
					selectionEnd.row = currentRow
					selectionEnd.col = currentCol
				}
			case 'h':
				if mode != 4 {
					if currentCol != 0 {
						currentCol--
					} else if currentRow > 0 {
						currentRow--
						currentCol = len(text_buffer[currentRow])
					}
				} else {
					if currentCol != 0 {
						currentCol--
					} else if currentRow > 0 {
						currentRow--
						currentCol = len(text_buffer[currentRow])
					}
					selectionEnd.row = currentRow
					selectionEnd.col = currentCol
				}

			case 'l':
				if mode != 4 {
					if currentCol != len(text_buffer[currentRow]) {
						currentCol++
					} else if currentRow < len(text_buffer)-1 {
						currentRow++
						currentCol = 0
					}
				} else {
					if currentCol != len(text_buffer[currentRow]) {
						currentCol++
					} else if currentRow < len(text_buffer)-1 {
						currentRow++
						currentCol = 0
					}
					selectionEnd.row = currentRow
					selectionEnd.col = currentCol
				}

			case 't':
				lineNumber := 1
				jumpToLine(&lineNumber)
			case 'b':
				lineNumber := len(text_buffer)
				jumpToLine(&lineNumber)
			case 'o':
				currentCol = len(text_buffer[currentRow])
				insert_line()
				modified = 0
				mode = 1
			}

			if currentCol > len(text_buffer[currentRow]) {
				currentCol = len(text_buffer[currentRow])
			}
			if currentCol < 0 {
				currentCol = 0
			}
		}
	} else {
		switch key_event.Key {
		case termbox.KeyCtrlS:
			write_file(source_file)
		case termbox.KeyEnter:
			if mode == 1 {
				insert_line()
				modified = 0
			} else {
				if currentRow < len(text_buffer)-1 {
					currentRow++
				}
			}
		case termbox.KeyBackspace:
			if mode == 1 {
				delete_rune()
				modified = 0
			} else {
				if currentCol != 0 {
					currentCol--
				} else if currentRow > 0 {
					currentRow--
					currentCol = len(text_buffer[currentRow])
				}
			}
		case termbox.KeyBackspace2:
			if mode == 1 {
				delete_rune()
				modified = 0
			} else {
				if currentCol != 0 {
					currentCol--
				} else if currentRow > 0 {
					currentRow--
					currentCol = len(text_buffer[currentRow])
				}
			}
		case termbox.KeyDelete:
			if mode == 1 {
				delete_right_rune()
				modified = 0
			} else {
				if currentCol != len(text_buffer[currentRow]) {
					currentCol++
				} else if currentRow < len(text_buffer)-1 {
					currentRow++
					currentCol = 0
				}
			}
		case termbox.KeyTab:
			if mode == 1 {
				for i := 0; i < tabWidth; i++ {
					insert_rune(key_event)
				}
				modified = 0
			}
		case termbox.KeySpace:
			if mode == 1 {
				insert_rune(key_event)
				modified = 0
			}
		case termbox.KeyHome:
			currentCol = 0
		case termbox.KeyEnd:
			currentCol = len(text_buffer[currentRow])
		case termbox.KeyPgup:
			if currentRow-int(ROWS/4) > 0 {
				currentRow -= int(ROWS / 4)
			}
		case termbox.KeyPgdn:
			if currentRow+int(ROWS/4) < len(text_buffer)-1 {
				currentRow += int(ROWS / 4)
			}
		case termbox.KeyArrowUp:
			if mode != 4 {
				if currentRow != 0 {
					currentRow--
				}
			} else {
				if currentRow != 0 {
					currentRow--
				}
				selectionEnd.row = currentRow
				selectionEnd.col = currentCol
			}
		case termbox.KeyArrowDown:
			if mode != 4 {
				if currentRow < len(text_buffer)-1 {
					currentRow++
				}
			} else {
				if currentRow < len(text_buffer)-1 {
					currentRow++
				}
				selectionEnd.row = currentRow
				selectionEnd.col = currentCol
			}
		case termbox.KeyArrowLeft:
			if mode != 4 {
				if currentCol != 0 {
					currentCol--
				} else if currentRow > 0 {
					currentRow--
					currentCol = len(text_buffer[currentRow])
				}
			} else {
				if currentCol != 0 {
					currentCol--
				} else if currentRow > 0 {
					currentRow--
					currentCol = len(text_buffer[currentRow])
				}
				selectionEnd.row = currentRow
				selectionEnd.col = currentCol
			}
		case termbox.KeyArrowRight:
			if mode != 4 {
				if currentCol != len(text_buffer[currentRow]) {
					currentCol++
				} else if currentRow < len(text_buffer)-1 {
					currentRow++
					currentCol = 0
				}
			} else {
				if currentCol != len(text_buffer[currentRow]) {
					currentCol++
				} else if currentRow < len(text_buffer)-1 {
					currentRow++
					currentCol = 0
				}
				selectionEnd.row = currentRow
				selectionEnd.col = currentCol
			}
		}
		if currentCol > len(text_buffer[currentRow]) {
			currentCol = len(text_buffer[currentRow])
		}
		if currentCol < 0 {
			currentCol = 0
		}
	}
}

func run_editor() {
	err := termbox.Init()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if len(os.Args) > 1 {
		source_file = os.Args[1]
		read_file(source_file)
	} else {
		source_file = "out.txt"
		text_buffer = append(text_buffer, []rune{})
	}

	modified = 1
	source_file2 = source_file
	source_file2 = strings.Replace(source_file, ".", "", 1)
	source_file2 = strings.ReplaceAll(source_file, "/", "")
	for {
		COLS, ROWS = termbox.Size()
		ROWS--
		if COLS < 78 {
			COLS = 78
		}
		termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
		scroll_text_buffer()
		display_text_buffer()
		display_status_bar()
		termbox.SetCursor(currentCol-offsetCol+lineNumberWidth, currentRow-offsetRow)
		termbox.Flush()
		process_key_press()
	}
}

func main() {
	run_editor()
}
