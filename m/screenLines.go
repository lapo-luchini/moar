package m

import (
	"fmt"

	"github.com/walles/moar/twin"
)

type renderedLine struct {
	inputLineOneBased int

	// If an input line has been wrapped into two, the part on the second line
	// will have a wrapIndex of 1.
	wrapIndex int

	cells []twin.Cell
}

func (p *Pager) redraw(spinner string) {
	p.screen.Clear()

	lastUpdatedScreenLineNumber := -1
	var renderedScreenLines [][]twin.Cell
	renderedScreenLines, statusText := p.renderScreenLines()
	for lineNumber, row := range renderedScreenLines {
		lastUpdatedScreenLineNumber = lineNumber
		for column, cell := range row {
			p.screen.SetCell(column, lastUpdatedScreenLineNumber, cell)
		}
	}

	eofSpinner := spinner
	if eofSpinner == "" {
		// This happens when we're done
		eofSpinner = "---"
	}
	spinnerLine := cellsFromString(_EofMarkerFormat + eofSpinner)
	for column, cell := range spinnerLine {
		p.screen.SetCell(column, lastUpdatedScreenLineNumber+1, cell)
	}

	switch p.mode {
	case _Searching:
		p.addSearchFooter()

	case _NotFound:
		p.setFooter("Not found: " + p.searchString)

	case _Viewing:
		helpText := "Press ESC / q to exit, '/' to search, '?' for help"
		if p.isShowingHelp {
			helpText = "Press ESC / q to exit help, '/' to search"
		}
		p.setFooter(statusText + spinner + "  " + helpText)

	default:
		panic(fmt.Sprint("Unsupported pager mode: ", p.mode))
	}

	p.screen.Show()
}

// Render screen lines into an array of lines consisting of Cells.
//
// The second return value is the same as firstInputLineOneBased, but decreased
// if needed so that the end of the input is visible.
func (p *Pager) renderScreenLines() (lines [][]twin.Cell, statusText string) {
	allPossibleLines, statusText := p.renderAllLines()
	if len(allPossibleLines) == 0 {
		return
	}

	// Find which index in allPossibleLines the user wants to see at the top of
	// the screen
	firstVisibleIndex := -1 // Not found
	for index, line := range allPossibleLines {
		if line.inputLineOneBased == p.firstLineOneBased {
			firstVisibleIndex = index
			break
		}
	}
	if firstVisibleIndex == -1 {
		panic(fmt.Errorf("firstInputLineOneBased %d not found in allPossibleLines size %d",
			p.firstLineOneBased, len(allPossibleLines)))
	}

	// Ensure the firstVisibleIndex is on an input line boundary, we don't
	// support by-screen-line positioning yet!
	for allPossibleLines[firstVisibleIndex].wrapIndex != 0 {
		firstVisibleIndex--
	}

	_, height := p.screen.Size()
	lastVisibleIndex := firstVisibleIndex + height - 2 // "-2" figured out through trial-and-error
	if lastVisibleIndex < len(allPossibleLines) {
		// Screen has enough room for everything, return everything
		lines, p.firstLineOneBased = p.toScreenLinesArray(allPossibleLines, firstVisibleIndex)
		return
	}

	// We seem to be too far down, clip!
	overshoot := 1 + lastVisibleIndex - len(allPossibleLines)
	firstVisibleIndex -= overshoot
	if firstVisibleIndex < 0 {
		firstVisibleIndex = 0
	}

	// Ensure the firstVisibleIndex is on an input line boundary, we don't
	// support by-screen-line positioning yet!
	for firstVisibleIndex > 0 && allPossibleLines[firstVisibleIndex].wrapIndex != 0 {
		firstVisibleIndex--
	}

	// Construct the screen lines to return
	lines, p.firstLineOneBased = p.toScreenLinesArray(allPossibleLines, firstVisibleIndex)
	return
}

func (p *Pager) toScreenLinesArray(allPossibleLines []renderedLine, firstVisibleIndex int) ([][]twin.Cell, int) {
	firstInputLineOneBased := allPossibleLines[firstVisibleIndex].inputLineOneBased

	_, height := p.screen.Size()
	screenLines := make([][]twin.Cell, 0, height)
	for index := firstVisibleIndex; ; index++ {
		if len(screenLines) >= height {
			// All lines rendered, done!
			break
		}
		if index >= len(allPossibleLines) {
			// No more lines available for rendering, done!
			break
		}
		screenLines = append(screenLines, allPossibleLines[index].cells)
	}

	return screenLines, firstInputLineOneBased
}

func (p *Pager) numberPrefixLength() int {
	if !p.ShowLineNumbers {
		return 0
	}

	// Count the length of the last line number
	numberPrefixLength := len(formatNumber(uint(p.getLastVisibleLineOneBased()))) + 1
	if numberPrefixLength < 4 {
		// 4 = space for 3 digits followed by one whitespace
		//
		// https://github.com/walles/moar/issues/38
		numberPrefixLength = 4
	}

	return numberPrefixLength
}

func (p *Pager) renderAllLines() ([]renderedLine, string) {
	_, height := p.screen.Size()
	wantedLineCount := height - 1

	if p.firstLineOneBased < 1 {
		p.firstLineOneBased = 1
	}
	inputLines := p.reader.GetLines(p.firstLineOneBased, wantedLineCount)
	if inputLines.lines == nil {
		// Empty input, empty output
		return []renderedLine{}, inputLines.statusText
	}

	// Offsets figured out through trial-and-error...
	lastInputLineOneBased := inputLines.firstLineOneBased + len(inputLines.lines) - 1
	if p.firstLineOneBased > lastInputLineOneBased {
		p.firstLineOneBased = lastInputLineOneBased
	}

	allLines := make([]renderedLine, 0)
	for lineIndex, line := range inputLines.lines {
		lineNumber := inputLines.firstLineOneBased + lineIndex

		allLines = append(allLines, p.renderLine(line, lineNumber)...)
	}

	return allLines, inputLines.statusText
}

// lineNumber and numberPrefixLength are required for knowing how much to
// indent, and to (optionally) render the line number.
func (p *Pager) renderLine(line *Line, lineNumber int) []renderedLine {
	highlighted := line.HighlightedTokens(p.searchPattern)
	var wrapped [][]twin.Cell
	if p.WrapLongLines {
		width, _ := p.screen.Size()
		wrapped = wrapLine(width-p.numberPrefixLength(), highlighted)
	} else {
		// All on one line
		wrapped = [][]twin.Cell{highlighted}
	}

	rendered := make([]renderedLine, 0)
	for wrapIndex, inputLinePart := range wrapped {
		visibleLineNumber := &lineNumber
		if wrapIndex > 0 {
			visibleLineNumber = nil
		}

		rendered = append(rendered, renderedLine{
			inputLineOneBased: lineNumber,
			wrapIndex:         wrapIndex,
			cells:             p.createScreenLine(visibleLineNumber, inputLinePart),
		})
	}

	return rendered
}

func (p *Pager) createScreenLine(lineNumberToShow *int, contents []twin.Cell) []twin.Cell {
	width, _ := p.screen.Size()
	newLine := make([]twin.Cell, 0, width)
	numberPrefixLength := p.numberPrefixLength()
	newLine = append(newLine, createLineNumberPrefix(lineNumberToShow, numberPrefixLength)...)

	startColumn := p.leftColumnZeroBased
	if startColumn < len(contents) {
		endColumn := p.leftColumnZeroBased + (width - numberPrefixLength)
		if endColumn > len(contents) {
			endColumn = len(contents)
		}
		newLine = append(newLine, contents[startColumn:endColumn]...)
	}

	// Add scroll left indicator
	if p.leftColumnZeroBased > 0 && len(contents) > 0 {
		if len(newLine) == 0 {
			// Don't panic on short lines, this new Cell will be
			// overwritten with '<' right after this if statement
			newLine = append(newLine, twin.Cell{})
		}

		// Add can-scroll-left marker
		newLine[0] = twin.Cell{
			Rune:  '<',
			Style: twin.StyleDefault.WithAttr(twin.AttrReverse),
		}
	}

	// Add scroll right indicator
	if len(contents)+numberPrefixLength-p.leftColumnZeroBased > width {
		newLine[width-1] = twin.Cell{
			Rune:  '>',
			Style: twin.StyleDefault.WithAttr(twin.AttrReverse),
		}
	}

	return newLine
}

// Generate a line number prefix. Can be empty or all-whitespace depending on parameters.
func createLineNumberPrefix(fileLineNumber *int, numberPrefixLength int) []twin.Cell {
	if numberPrefixLength == 0 {
		return []twin.Cell{}
	}

	lineNumberPrefix := make([]twin.Cell, 0, numberPrefixLength)
	if fileLineNumber == nil {
		for len(lineNumberPrefix) < numberPrefixLength {
			lineNumberPrefix = append(lineNumberPrefix, twin.Cell{Rune: ' '})
		}
		return lineNumberPrefix
	}

	lineNumberString := formatNumber(uint(*fileLineNumber))
	lineNumberString = fmt.Sprintf("%*s ", numberPrefixLength-1, lineNumberString)
	if len(lineNumberString) > numberPrefixLength {
		panic(fmt.Errorf(
			"lineNumberString <%s> longer than numberPrefixLength %d",
			lineNumberString, numberPrefixLength))
	}

	for column, digit := range lineNumberString {
		if column >= numberPrefixLength {
			break
		}

		lineNumberPrefix = append(lineNumberPrefix, twin.NewCell(digit, _numberStyle))
	}

	return lineNumberPrefix
}

func (p *Pager) getLastVisiblePosition() scrollPosition {
	// FIXME: Compute this!
}

// Is the given position visible on screen?
func (p *Pager) isVisible(scrollPosition scrollPosition) bool {
	// FIXME: Compute this!
}
