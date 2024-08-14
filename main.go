package main

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/mattn/go-runewidth"
	"github.com/nsf/termbox-go"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

var (
	ROWS, COLS             int
	offsetRow, offsetCol   int
	currentCol, currentRow int
	source_file            string
	mode                   int
	text_buffer            = [][]rune{}
)

type EditorState struct {
	buffer    [][]rune
	cursorRow int
	cursorCol int
	offsetRow int
	offsetCol int
}

var (
	undoStack []EditorState
	redoStack []EditorState
)

const maxUndoLevels = 100

var (
	copy_buffer      = []rune{}
	modified         int
	searchHighlights []struct {
		row, startCol, endCol int
	}
)
var searchQuery string

const (
	lineNumberWidth = 5
	tabWidth        = 2
)

var (
	file_extension string
	parentDir      string
	homeDir        string
)

var keywords = map[string]bool{
	"func":    true,
	"var":     true,
	"const":   true,
	"if":      true,
	"else":    true,
	"for":     true,
	"return":  true,
	"import":  true,
	"package": true,
	"type":    true,
	"struct":  true,
	"switch":  true,
	"case":    true,
}

var primitives = map[string]bool{
	"int":     true,
	"int8":    true,
	"int32":   true,
	"int64":   true,
	"string":  true,
	"bool":    true,
	"boolean": true,
	"rune":    true,
	"true":    true,
	"false":   true,
}

var operators = map[rune]bool{
	'+': true,
	'-': true,
	'*': true,
	'/': true,
	'=': true,
	'>': true,
	'<': true,
	'!': true,
	';': true,
	':': true,
	'.': true,
}

var ints = map[rune]bool{
	0: true, 1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true, 9: true,
}

func isKeyword(word string) bool {
	_, found := keywords[word]
	return found
}

func isPrimitive(word string) bool {
	_, found := primitives[word]
	return found
}

func isOperator(ch rune) bool {
	_, found := operators[ch]
	return found
}

func isInt(ch rune) bool {
	_, found := ints[ch]
	return found
}

func findText() {
	searchHighlights = []struct{ row, startCol, endCol int }{}
	searchQuery = ""

	mode = 2 // Assuming 2 is the mode for searching

	// Initialize index for navigating through search highlights
	highlightIndex := 0

	for {
		termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
		display_text_buffer()
		print_message(0, ROWS, termbox.ColorBlack, termbox.ColorWhite, " "+string('\ue23e')+" "+string('\uf002')+" SEARCH: "+searchQuery)
		termbox.SetCursor(len("SEARCH: ")+len(searchQuery)+5, ROWS)
		termbox.Flush()

		ev := termbox.PollEvent()
		switch ev.Type {
		case termbox.EventKey:
			switch ev.Key {
			case termbox.KeyEsc:
				mode = 0
				searchHighlights = []struct{ row, startCol, endCol int }{}
				return
			case termbox.KeyEnter:
				if len(searchHighlights) > 0 {
					// Navigate to the current search highlight
					if highlightIndex >= len(searchHighlights) {
						highlightIndex = 0 // Loop back to the start if at the end
					}
					currentRow = searchHighlights[highlightIndex].row
					currentCol = searchHighlights[highlightIndex].startCol
					lineToJump := currentRow + 1
					jumpToLine(&lineToJump)
					highlightIndex++
					searchHighlights = []struct{ row, startCol, endCol int }{}
					for i, line := range text_buffer {
						lineStr := string(line)
						index := 0
						for {
							startIndex := strings.Index(lineStr[index:], searchQuery)
							if startIndex == -1 {
								break
							}
							startIndex += index
							endIndex := startIndex + len(searchQuery)
							searchHighlights = append(searchHighlights, struct{ row, startCol, endCol int }{i, startIndex, endIndex})
							index = endIndex
						}
					}
				}
				// Do not change mode here; stay in search mode
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

		// Perform search only if searchQuery is not empty

		if searchQuery != "" {
			searchHighlights = []struct{ row, startCol, endCol int }{}
			// Convert searchQuery to lower case for case-insensitive comparison
			lowerSearchQuery := strings.ToLower(searchQuery)

			for i, line := range text_buffer {
				lineStr := string(line)
				// Convert lineStr to lower case for case-insensitive comparison
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
		} else {
			searchHighlights = []struct{ row, startCol, endCol int }{} // Clear highlights if search query is empty
		}
	}
}

func read_file(filename string) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting current directory:", err)
		return
	}

	fullPath := filepath.Join(cwd, filename)
	file_extension = strings.Split(filename, ".")[1]
	parentDir = filepath.Base(filepath.Dir(fullPath))
	dirname, err := os.UserHomeDir()
	if err != nil {
		fmt.Println(err)
	}
	homeDir = dirname

	file, err := os.Open(filename)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	// Open a file for writing the rune output
	outputFile := "randomFile.txt" // You can set this to any desired filename
	outFile, err := os.Create(outputFile)
	if err != nil {
		fmt.Println("Error creating output file:", err)
		return
	}
	defer outFile.Close()

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

			// Write the rune and its Unicode representation to the output file
			fmt.Fprintf(outFile, "Rune: %c, Unicode: \\u{%X}\n", r, r)
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
		insert_rune[currentCol] = rune(' ')
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

func paste_line() {
	push_buffer()
	if len(copy_buffer) == 0 {
		currentRow++
		currentCol = 0
	}
	new_text_buffer := make([][]rune, len(text_buffer)+1)
	copy(new_text_buffer[:currentRow], text_buffer[:currentRow])
	new_text_buffer[currentRow] = copy_buffer
	copy(new_text_buffer[currentRow+1:], text_buffer[currentRow:])
	text_buffer = new_text_buffer
}

func paste_line_below() {
	push_buffer()
	if len(copy_buffer) != 0 {
		if currentRow < len(text_buffer) {
			currentRow++
		} else {
			currentRow = len(text_buffer)
		}
		currentCol = 0

		new_text_buffer := make([][]rune, len(text_buffer)+1)
		copy(new_text_buffer[:currentRow], text_buffer[:currentRow])
		new_text_buffer[currentRow] = make([]rune, len(copy_buffer))
		copy(new_text_buffer[currentRow], copy_buffer)
		copy(new_text_buffer[currentRow+1:], text_buffer[currentRow:])

		text_buffer = new_text_buffer
	}
}

func cut_line() {
	push_buffer()
	copy_line()
	if currentRow >= len(text_buffer) || len(text_buffer) < 2 {
		return
	}

	new_text_buffer := make([][]rune, len(text_buffer)-1)
	copy(new_text_buffer[:currentRow], text_buffer[:currentRow])
	copy(new_text_buffer[currentRow:], text_buffer[currentRow+1:])
	text_buffer = new_text_buffer

	if currentRow > 0 {
		currentRow--
		currentCol = 0
	}
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

func redo_buffer() {
	if len(redoStack) == 0 {
		return
	}
	// Push current state to undoStack before redoing
	push_buffer()

	// Restore the last state from the redo stack
	lastState := redoStack[len(redoStack)-1]
	text_buffer = lastState.buffer
	currentRow = lastState.cursorRow
	currentCol = lastState.cursorCol
	offsetRow = lastState.offsetRow
	offsetCol = lastState.offsetCol

	redoStack = redoStack[:len(redoStack)-1]
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
		print_message(0, ROWS, termbox.ColorBlack, termbox.ColorWhite, " "+string('\ue23e')+" Jump to line: "+lineNumberStr)
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
	utf8Encoder := unicode.UTF8.NewEncoder()
	writer := transform.NewWriter(file, utf8Encoder)

	for _, line := range text_buffer {
		_, err = writer.Write([]byte(string(line) + "\n"))
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return
		}
	}

	err = writer.Close()
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
	inString := false          // Track if we're inside a string
	stringDelimiter := rune(0) // To track the current string delimiter
	inComment := false         // Track if we're inside a comment
	bg := termbox.ColorDefault

	for row = 0; row < ROWS; row++ {
		text_buffer_row := row + offsetRow

		// Display line number
		lineNumber := fmt.Sprintf("%*d", lineNumberWidth-1, text_buffer_row+1)
		if currentRow == text_buffer_row {
			for i, ch := range lineNumber {
				termbox.SetCell(i, row, ch, termbox.ColorLightGray, termbox.ColorDefault)
			}
			termbox.SetCell(lineNumberWidth-1, row, '\u2502', termbox.ColorLightGray, termbox.ColorDefault)
		} else {
			for i, ch := range lineNumber {
				termbox.SetCell(i, row, ch, termbox.ColorBlack, termbox.ColorDefault)
			}
			termbox.SetCell(lineNumberWidth-1, row, '\u2502', termbox.ColorBlack, termbox.ColorDefault)
		}

		if text_buffer_row < len(text_buffer) {
			line := text_buffer[text_buffer_row]
			word := ""
			visibleCol := 0 // Track the visible column on the screen

			// Initialize `inComment` flag
			inComment = false

			// Determine the initial `inString` state based on the portion of the line before the visible area
			inString, stringDelimiter = determineInStringState(string(line), offsetCol, inString, stringDelimiter)

			for col = 0; col < COLS-lineNumberWidth; col++ {
				text_buffer_column := col + offsetCol

				if text_buffer_column < len(line) {
					ch := line[text_buffer_column]
					bg = termbox.ColorDefault
					highlighted := false
					for _, highlight := range searchHighlights {
						if highlight.row == text_buffer_row &&
							text_buffer_column >= highlight.startCol &&
							text_buffer_column < highlight.endCol {
							highlighted = true
							bg = termbox.ColorYellow
							break
						}
					}

					if inComment {
						termbox.SetCell(visibleCol+lineNumberWidth, row, ch, termbox.ColorBlack, bg)
						visibleCol++
						if ch == '\n' {
							inComment = false // End of line, exit comment mode
						}
					} else if inString {
						// Handle escape characters in string
						if ch == '\\' && text_buffer_column+1 < len(line) && line[text_buffer_column+1] == stringDelimiter {
							termbox.SetCell(visibleCol+lineNumberWidth, row, ch, termbox.ColorGreen, bg)
							visibleCol++
							text_buffer_column++
							ch = line[text_buffer_column]
						}

						// Highlight string
						termbox.SetCell(visibleCol+lineNumberWidth, row, ch, termbox.ColorGreen, bg)
						visibleCol++

						if ch == stringDelimiter {
							inString = false
							stringDelimiter = rune(0)
						}
					} else {
						if ch == '/' && text_buffer_column+1 < len(line) && line[text_buffer_column+1] == '/' {
							inComment = true
							termbox.SetCell(visibleCol+lineNumberWidth, row, ch, termbox.ColorBlack, bg)
							visibleCol++
							text_buffer_column++ // Skip the next character '/'
						} else if ch == '"' || ch == '\'' || ch == '`' {
							inString = true
							stringDelimiter = rune(ch) // Set the current delimiter
							termbox.SetCell(visibleCol+lineNumberWidth, row, ch, termbox.ColorGreen, bg)
							visibleCol++
						} else if isOperator(ch) {
							termbox.SetCell(visibleCol+lineNumberWidth, row, ch, termbox.ColorRed, bg)
							visibleCol++
						} else if isInt(ch) {
							termbox.SetCell(visibleCol+lineNumberWidth, row, ch, termbox.ColorCyan, bg)
							visibleCol++

						} else if ch == ' ' || ch == '{' || ch == '(' || ch == '\u0085' || ch == '\u2028' || ch == '\u2029' || ch == '}' || ch == '\u0030' || ch == '\u0033' || ch == ')' {
							if len(word) > 0 {
								if isKeyword(word) {
									for i := 0; i < len(word); i++ {
										termbox.SetCell(visibleCol+lineNumberWidth-len(word)+i, row, rune(word[i]), termbox.ColorMagenta, bg)
									}
								} else if isPrimitive(word) {
									for i := 0; i < len(word); i++ {
										termbox.SetCell(visibleCol+lineNumberWidth-len(word)+i, row, rune(word[i]), termbox.ColorYellow, bg)
									}
								}
								word = ""
							}
							termbox.SetCell(visibleCol+lineNumberWidth, row, ch, termbox.ColorDefault, bg)
							visibleCol++
						} else if ch == '\t' {
							// Expand the tab into spaces
							spaceCount := tabWidth - (visibleCol % tabWidth)
							for i := 0; i < spaceCount; i++ {
								termbox.SetCell(visibleCol+lineNumberWidth, row, ' ', termbox.ColorDefault, bg)
								visibleCol++
							}
						} else {
							word += string(ch)
							if highlighted {
								termbox.SetCell(visibleCol+lineNumberWidth, row, ch, termbox.ColorBlack, termbox.ColorYellow)
							} else {
								termbox.SetCell(visibleCol+lineNumberWidth, row, ch, termbox.ColorDefault, bg)
							}
							visibleCol++
						}
					}
				} else {
					break
				}
			}

			if inString {
				inString = false
				stringDelimiter = rune(0)
			}
			if len(word) > 0 {
				if isKeyword(word) {
					for i := 0; i < len(word); i++ {
						termbox.SetCell(visibleCol+lineNumberWidth-len(word)+i, row, rune(word[i]), termbox.ColorMagenta, bg)
					}
				} else if isPrimitive(word) {
					for i := 0; i < len(word); i++ {
						termbox.SetCell(visibleCol+lineNumberWidth-len(word)+i, row, rune(word[i]), termbox.ColorYellow, bg)
					}
				}
			}
		}
	}
}

func determineInStringState(line string, offsetCol int, previousInString bool, prevDelimiter rune) (bool, rune) {
	inString := previousInString
	delimiter := prevDelimiter
	for i := 0; i < offsetCol && i < len(line); i++ {
		if line[i] == '"' || line[i] == '\'' || line[i] == '\u0060' {
			if delimiter == 0 {
				inString = !inString
				delimiter = rune(line[i])
			} else if rune(line[i]) == delimiter {
				inString = !inString
				delimiter = 0
			}
		} else if line[i] == '\\' && i+1 < len(line) && rune(line[i+1]) == delimiter {
			i++ // Skip the escaped quote
		}
	}
	return inString, delimiter
}

func display_status_bar() {
	var mode_status string
	var file_status string
	var copy_status string
	var undo_status string
	var logo rune

	if mode == 1 {
		mode_status = " " + string('\ue23e') + " INSERT: "
	} else if mode == 2 {
		mode_status = " " + string('\ue23e') + " " + string('\uf002') + " SEARCH: "
	} else if mode == 3 {
		mode_status = " " + string('\ue23e') + " JUMP TO: "
	} else {
		mode_status = " " + string('\ue23e') + " NORMAL: "
	}

	filename_length := len(source_file)
	switch file_extension {
	case "astro":
		logo = '\ue6b3'
	case "asm":
		logo = '\ueae8'
	case "bat":
		logo = ''
	case "bash":
		logo = ''
	case "c":
		logo = '\ue61e'
	case "cs":
		logo = '\ue648'
	case "cpp":
		logo = ''
	case "css":
		logo = ''
	case "csv":
		logo = ''
	case "cmake":
		logo = ''
	case "dart":
		logo = ''
	case "html":
		logo = ''
	case "hpp":
		logo = ''
	case "go":
		logo = ''
	case "jsx":
		logo = ''
	case "tsx":
		logo = ''
	case "java":
		logo = ''
	case "lua":
		logo = ''
	case "md":
		logo = ''
	case "php":
		logo = '\ue73d'
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
		logo = '\ue739'
	case "sh":
		logo = ''
	case "gitignore":
		logo = '\ue702'
	case "sql":
		logo = ''
	case "sqlite":
		logo = '\ue7c4'
	case "db":
		logo = '\ue64d'
	case "swift":
		logo = '\ue755'
	case "toml":
		logo = ''
	case "txt":
		logo = '\uf15b'
	case "scss":
		logo = ''
	case "sass":
		logo = ''
	case "ts":
		logo = ''
	case "exe":
		logo = '\ueae8'
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
		logo = '\uf15b'
	}

	if filename_length > 13 {
		filename_length = 13
	}

	if len(text_buffer) > 1 {
		file_status = source_file[:filename_length] + " " + string(logo) + " . " + strconv.Itoa(len(text_buffer)) + " lines"
	} else {
		file_status = source_file[:filename_length] + " " + string(logo) + " . " + strconv.Itoa(len(text_buffer)) + " line"
	}
	if modified == 0 {
		file_status += " modified"
	} else if modified == 1 {
		file_status += " oldest change"
	} else if modified == 2 {
		file_status += " saved"
	}
	var parent_status string
	homeDir = path.Base(filepath.Dir(homeDir))

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
	used_space := len(mode_status) + len(file_status) + len(copy_status) + len(undo_status) + len(file_percent) + len(parent_status)
	spaces := strings.Repeat(" ", COLS-used_space)
	message := mode_status + file_status + copy_status + undo_status + spaces + parent_status + file_percent
	print_message(0, ROWS, termbox.ColorBlack, termbox.ColorWhite, message)
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
			print_message(0, ROWS, termbox.ColorBlack, termbox.ColorWhite, " Would you like to save before leaving(y/n): "+answer)
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
			case 'w':
				write_file(source_file)
			case 'y':
				copy_line()
				modified = 0
			case 'P':
				paste_line()
				modified = 0
			case 'd':
				cut_line()
				modified = 0
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
				if currentRow != 0 {
					currentRow--
				}
			case 'j':
				if currentRow < len(text_buffer)-1 {
					currentRow++
				}
			case 'h':
				if currentCol != 0 {
					currentCol--
				} else if currentRow > 0 {
					currentRow--
					currentCol = len(text_buffer[currentRow])
				}
			case 'l':
				if currentCol != len(text_buffer[currentRow]) {
					currentCol++
				} else if currentRow < len(text_buffer)-1 {
					currentRow++
					currentCol = 0
				}
			case 't':
				lineNumber := 1
				jumpToLine(&lineNumber)
			case 'b':
				lineNumber := len(text_buffer)
				jumpToLine(&lineNumber)
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
			delete_rune()
			modified = 0
		case termbox.KeyBackspace2:
			delete_rune()
			modified = 0
		case termbox.KeyDelete:
			delete_right_rune()
			modified = 0
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
			if currentRow != 0 {
				currentRow--
			}
		case termbox.KeyArrowDown:
			if currentRow < len(text_buffer)-1 {
				currentRow++
			}
		case termbox.KeyArrowLeft:
			if currentCol != 0 {
				currentCol--
			} else if currentRow > 0 {
				currentRow--
				currentCol = len(text_buffer[currentRow])
			}
		case termbox.KeyArrowRight:
			if currentCol != len(text_buffer[currentRow]) {
				currentCol++
			} else if currentRow < len(text_buffer)-1 {
				currentRow++
				currentCol = 0
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
