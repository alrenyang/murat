package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"go.bug.st/serial"
)

const configFile = "muart_config.json"

const (
	maxDisplayLogLines   = 5000
	serialReadBufferSize = 4096
)

type Config struct {
	PortName       string          `json:"port_name"`
	BaudRate       int             `json:"baud_rate"`
	LineEnding     string          `json:"line_ending"`
	ShowTimestamp  bool            `json:"show_timestamp"`
	AutoScroll     bool            `json:"auto_scroll"`
	IsHexMode      bool            `json:"is_hex_mode"`
	Theme          int             `json:"theme"`
	FontFamily     string          `json:"font_family"`
	FontSize       int             `json:"font_size"`
	LogDirPath     string          `json:"log_dir_path"`
	HighlightRules []HighlightRule `json:"highlight_rules"`
}

func loadConfig() Config {
	defaultCfg := Config{
		PortName:       "/dev/tty.usbserial-0001",
		BaudRate:       115200,
		LineEnding:     "CRLF",
		ShowTimestamp:  false,
		AutoScroll:     true,
		IsHexMode:      false,
		Theme:          0,
		FontFamily:     "JetBrains Mono",
		FontSize:       14,
		LogDirPath:     ".",
		HighlightRules: []HighlightRule{},
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return defaultCfg
	}

	cfg := defaultCfg
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultCfg
	}
	if cfg.BaudRate <= 0 {
		cfg.BaudRate = defaultCfg.BaudRate
	}
	return cfg
}

func (m *model) saveConfig() error {
	cfg := Config{
		PortName:       m.portName,
		BaudRate:       m.baudRate,
		LineEnding:     m.lineEnding,
		ShowTimestamp:  m.showTimestamp,
		AutoScroll:     m.autoScroll,
		IsHexMode:      m.isHexMode,
		Theme:          m.theme,
		FontFamily:     m.fontFamily,
		FontSize:       m.fontSize,
		LogDirPath:     m.logDirPath,
		HighlightRules: m.highlightRules,
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := configFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, configFile)
}

func (m *model) persistConfig() {
	if err := m.saveConfig(); err != nil {
		m.writeSysLog(fmt.Sprintf("[에러] 설정 파일 저장 실패: %v", err))
	}
}

type serialMsg struct {
	data       []byte
	err        error
	generation uint64
}

type HighlightRule struct {
	Keyword string `json:"keyword"`
	Color   string `json:"color"`
}

type ThemeStyles struct {
	AppBorder lipgloss.Style
	Box       lipgloss.Style
	Footer    lipgloss.Style
	Title     lipgloss.Style
	Help      lipgloss.Style
	Modal     lipgloss.Style
}

func getThemeStyles(theme int) ThemeStyles {
	switch theme {
	case 1:
		return ThemeStyles{
			AppBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("33")),
			Box:       lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("51")).Padding(0, 1),
			Footer:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("75")).Padding(0, 1),
			Title:     lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true).Italic(true),
			Help:      lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
			Modal:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39")).Padding(1, 3).Width(55),
		}
	case 2:
		return ThemeStyles{
			AppBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("208")),
			Box:       lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("214")).Padding(0, 1),
			Footer:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("202")).Padding(0, 1),
			Title:     lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Italic(true),
			Help:      lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
			Modal:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("208")).Padding(1, 3).Width(55),
		}
	default:
		return ThemeStyles{
			AppBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("99")),
			Box:       lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("42")).Padding(0, 1),
			Footer:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("39")).Padding(0, 1),
			Title:     lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Italic(true),
			Help:      lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
			Modal:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("214")).Padding(1, 3).Width(55),
		}
	}
}

type model struct {
	port           serial.Port
	portGeneration uint64
	portName       string
	baudRate       int
	lineEnding     string
	viewport       viewport.Model
	memoArea       textarea.Model
	textInput      textinput.Model
	activePane     int

	showSplit bool
	showHelp  bool

	displayLogLines []string
	newLineStarted  bool

	searchQuery string
	width       int
	height      int

	rxBytes int64
	rxPkts  int64
	txBytes int64
	txPkts  int64

	selectingStep  int
	availablePorts []string
	portCursor     int

	baudRates    []int
	baudCursor   int
	tempPortName string

	lineEndings   []string
	lineEndCursor int

	memoFiles  []string
	memoCursor int

	showTimestamp  bool
	autoScroll     bool
	isLogging      bool
	isHexMode      bool
	theme          int
	fontFamily     string
	fontSize       int
	logDirPath     string
	logFile        *os.File
	isTerminalMode bool
	settingCursor  int

	highlightRules []HighlightRule
	hlCursor       int
	hlInputMode    bool
	hlInputStep    int
	tempKeyword    string
}

func getColorStyle(colorName string) lipgloss.Style {
	switch strings.ToLower(colorName) {
	case "red":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	case "green":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true)
	case "yellow":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	case "blue":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	case "cyan":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	}
}

func highlightLog(line string, rules []HighlightRule) string {
	upper := strings.ToUpper(line)
	for _, rule := range rules {
		if rule.Keyword != "" && strings.Contains(upper, strings.ToUpper(rule.Keyword)) {
			st := getColorStyle(rule.Color)
			return st.Render(line)
		}
	}
	return line
}

func (m *model) writeSysLog(text string) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render(text)
	m.displayLogLines = append(m.displayLogLines, styled, "")
	m.newLineStarted = true
	m.updateViewportContent()
}

func (m *model) processIncomingBytes(data []byte) {
	for _, b := range data {
		lastIdx := len(m.displayLogLines) - 1

		if m.isLogging && m.logFile != nil {
			m.logFile.Write([]byte{b})
		}

		if m.isHexMode {
			hexStr := fmt.Sprintf("%02X ", b)
			m.displayLogLines[lastIdx] += hexStr
			if len(m.displayLogLines[lastIdx]) > 80 {
				m.displayLogLines = append(m.displayLogLines, "")
			}
			continue
		}

		if b == '\n' {
			m.displayLogLines = append(m.displayLogLines, "")
			m.newLineStarted = true
		} else if b == '\r' {
			continue
		} else if b == '\b' || b == '\x7f' {
			if len(m.displayLogLines[lastIdx]) > 0 {
				m.displayLogLines[lastIdx] = m.displayLogLines[lastIdx][:len(m.displayLogLines[lastIdx])-1]
			}
		} else {
			if m.newLineStarted && m.showTimestamp {
				ts := time.Now().Format("15:04:05.000")
				tsStr := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(fmt.Sprintf("[%s] ", ts))
				m.displayLogLines[lastIdx] += tsStr
				m.newLineStarted = false
			}
			m.displayLogLines[lastIdx] += string(b)
		}
	}

	if len(m.displayLogLines) > maxDisplayLogLines {
		m.displayLogLines = m.displayLogLines[len(m.displayLogLines)-maxDisplayLogLines:]
	}
	m.updateViewportContent()
}

func (m *model) updateViewportContent() {
	var contentBuilder strings.Builder
	for i, line := range m.displayLogLines {
		hlLine := highlightLog(line, m.highlightRules)
		contentBuilder.WriteString(hlLine)
		if i < len(m.displayLogLines)-1 {
			contentBuilder.WriteString("\n")
		}
	}

	if m.isTerminalMode {
		contentBuilder.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render("█"))
	}

	m.viewport.SetContent(contentBuilder.String())
	if m.autoScroll {
		m.viewport.GotoBottom()
	}
}

func keyToBytes(msg tea.KeyMsg) []byte {
	if msg.Type == tea.KeyRunes {
		return []byte(string(msg.Runes))
	}
	switch msg.Type {
	case tea.KeyEnter:
		return []byte{'\r'}
	case tea.KeyTab:
		return []byte{'\t'}
	case tea.KeySpace:
		return []byte{' '}
	case tea.KeyBackspace, tea.KeyDelete:
		return []byte{'\x7f'}
	case tea.KeyEsc:
		return []byte{'\x1b'}
	case tea.KeyUp:
		return []byte("\x1b[A")
	case tea.KeyDown:
		return []byte("\x1b[B")
	case tea.KeyRight:
		return []byte("\x1b[C")
	case tea.KeyLeft:
		return []byte("\x1b[D")
	case tea.KeyCtrlB:
		return []byte{'\x02'}
	case tea.KeyCtrlC:
		return []byte{'\x03'}
	case tea.KeyCtrlD:
		return []byte{'\x04'}
	case tea.KeyCtrlE:
		return []byte{'\x05'}
	case tea.KeyCtrlF:
		return []byte{'\x06'}
	case tea.KeyCtrlG:
		return []byte{'\x07'}
	case tea.KeyCtrlH:
		return []byte{'\x08'}
	case tea.KeyCtrlJ:
		return []byte{'\x0a'}
	case tea.KeyCtrlK:
		return []byte{'\x0b'}
	case tea.KeyCtrlL:
		return []byte{'\x0c'}
	case tea.KeyCtrlN:
		return []byte{'\x0e'}
	case tea.KeyCtrlO:
		return []byte{'\x0f'}
	case tea.KeyCtrlP:
		return []byte{'\x10'}
	case tea.KeyCtrlQ:
		return []byte{'\x11'}
	case tea.KeyCtrlR:
		return []byte{'\x12'}
	case tea.KeyCtrlS:
		return []byte{'\x13'}
	case tea.KeyCtrlT:
		return []byte{'\x14'}
	case tea.KeyCtrlU:
		return []byte{'\x15'}
	case tea.KeyCtrlV:
		return []byte{'\x16'}
	case tea.KeyCtrlW:
		return []byte{'\x17'}
	case tea.KeyCtrlX:
		return []byte{'\x18'}
	case tea.KeyCtrlY:
		return []byte{'\x19'}
	case tea.KeyCtrlZ:
		return []byte{'\x1a'}
	}
	return nil
}

func readSerial(p serial.Port, generation uint64) tea.Cmd {
	return func() tea.Msg {
		buf := make([]byte, serialReadBufferSize)
		n, err := p.Read(buf)
		msg := serialMsg{generation: generation, err: err}
		if n > 0 {
			msg.data = append([]byte(nil), buf[:n]...)
		}
		return msg
	}
}

func (m model) Init() tea.Cmd {
	if m.port != nil {
		return tea.Batch(readSerial(m.port, m.portGeneration), textinput.Blink)
	}
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.MouseMsg:
		if m.selectingStep == 0 {
			switch msg.Type {
			case tea.MouseWheelUp:
				m.viewport.LineUp(3)
				m.autoScroll = false
				return m, nil
			case tea.MouseWheelDown:
				m.viewport.LineDown(3)
				if m.viewport.AtBottom() {
					m.autoScroll = true
				}
				return m, nil
			}
		}

	case tea.KeyMsg:
		// 1. [터미널 모드]일 때: 입력하는 모든 키가 장비로 즉시 전송됨 (Esc 누르면 메뉴 모드로 탈출)
		if m.isTerminalMode && m.selectingStep == 0 {
			if msg.Type == tea.KeyEsc {
				m.isTerminalMode = false
				m.writeSysLog("\n[시스템] 메뉴 모드 진입 (터미널 복귀: Esc 또는 i | 설정: s | 종료: q)")
				return m, nil
			}

			if m.port != nil {
				b := keyToBytes(msg)
				if len(b) > 0 {
					_, err := m.port.Write(b)
					if err != nil {
						m.writeSysLog(fmt.Sprintf("\n[송신 에러] %v", err))
						_ = m.port.Close()
						m.port = nil
					} else {
						m.txBytes += int64(len(b))
						m.txPkts++
					}
				}
			} else {
				m.writeSysLog("\n[에러] 연결된 포트가 없습니다. 메뉴 모드에서 'p'를 눌러 포트를 선택하세요.")
				m.isTerminalMode = false
			}
			return m, nil
		}

		// 2. [모달/팝업(Selecting Step)] 처리
		if m.showHelp {
			switch msg.String() {
			case "?", "esc", "q":
				m.showHelp = false
			}
			return m, nil
		}

		if m.selectingStep == 10 {
			switch msg.String() {
			case "esc", "q":
				m.selectingStep = 0
			case "up", "k":
				if m.memoCursor > 0 {
					m.memoCursor--
				}
			case "down", "j":
				if m.memoCursor < len(m.memoFiles)-1 {
					m.memoCursor++
				}
			case "enter":
				if len(m.memoFiles) > 0 {
					selectedFile := filepath.Join(m.logDirPath, m.memoFiles[m.memoCursor])
					data, err := os.ReadFile(selectedFile)
					if err == nil {
						m.memoArea.SetValue(string(data))
						m.writeSysLog(fmt.Sprintf("[메모] 불러오기 성공: %s", selectedFile))
					}
				}
				m.selectingStep = 0
			}
			return m, nil
		}

		if m.selectingStep == 1 {
			switch msg.String() {
			case "esc", "q":
				m.selectingStep = 0
			case "up", "k":
				if m.portCursor > 0 {
					m.portCursor--
				}
			case "down", "j":
				if m.portCursor < len(m.availablePorts)-1 {
					m.portCursor++
				}
			case "enter":
				if len(m.availablePorts) > 0 {
					m.tempPortName = m.availablePorts[m.portCursor]
					m.selectingStep = 2
				} else {
					m.selectingStep = 0
				}
			}
			return m, nil
		}

		if m.selectingStep == 2 {
			switch msg.String() {
			case "esc", "q":
				m.selectingStep = 0
			case "up", "k":
				if m.baudCursor > 0 {
					m.baudCursor--
				}
			case "down", "j":
				if m.baudCursor < len(m.baudRates)-1 {
					m.baudCursor++
				}
			case "enter":
				m.selectingStep = 9
				m.lineEndCursor = 0
				for i, le := range m.lineEndings {
					if le == m.lineEnding {
						m.lineEndCursor = i
						break
					}
				}
			}
			return m, nil
		}

		if m.selectingStep == 9 {
			switch msg.String() {
			case "esc", "q":
				m.selectingStep = 0
			case "up", "k":
				if m.lineEndCursor > 0 {
					m.lineEndCursor--
				}
			case "down", "j":
				if m.lineEndCursor < len(m.lineEndings)-1 {
					m.lineEndCursor++
				}
			case "enter":
				selectedBaud := m.baudRates[m.baudCursor]
				selectedLineEnding := m.lineEndings[m.lineEndCursor]

				if m.port != nil {
					_ = m.port.Close()
					m.port = nil
				}
				m.portGeneration++
				m.portName = m.tempPortName
				m.baudRate = selectedBaud
				m.lineEnding = selectedLineEnding

				mode := &serial.Mode{BaudRate: m.baudRate, Parity: serial.NoParity, DataBits: 8, StopBits: serial.OneStopBit}
				p, err := serial.Open(m.portName, mode)
				if err != nil {
					m.writeSysLog(fmt.Sprintf("[에러] 포트 열기 실패: %v", err))
				} else {
					m.port = p
					m.writeSysLog(fmt.Sprintf("[설정 완료] 포트: %s | 속도: %d | 줄끝: %s", m.portName, m.baudRate, m.lineEnding))
				}
				m.persistConfig()
				m.selectingStep = 0
				m.isTerminalMode = true
				if m.port != nil {
					return m, readSerial(m.port, m.portGeneration)
				}
			}
			return m, nil
		}

		if m.selectingStep == 3 {
			switch msg.String() {
			case "esc", "q":
				m.selectingStep = 0
			case "up", "k":
				if m.settingCursor > 0 {
					m.settingCursor--
				}
			case "down", "j":
				if m.settingCursor < 7 {
					m.settingCursor++
				}
			case "enter":
				if m.settingCursor == 0 {
					m.showTimestamp = !m.showTimestamp
				} else if m.settingCursor == 1 {
					m.autoScroll = !m.autoScroll
					if m.autoScroll {
						m.viewport.GotoBottom()
					}
				} else if m.settingCursor == 2 {
					if m.isLogging {
						if m.logFile != nil {
							_ = m.logFile.Close()
							m.logFile = nil
						}
						m.isLogging = false
					} else {
						_ = os.MkdirAll(m.logDirPath, 0755)
						filename := filepath.Join(m.logDirPath, fmt.Sprintf("muart_log_%s.txt", time.Now().Format("20060102_150405")))
						f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
						if err == nil {
							m.logFile = f
							m.isLogging = true
						}
					}
				} else if m.settingCursor == 3 {
					m.isHexMode = !m.isHexMode
				} else if m.settingCursor == 4 {
					m.lineEnding = m.lineEndings[(m.settingCursor+1)%len(m.lineEndings)]
				} else if m.settingCursor == 5 {
					m.theme = (m.theme + 1) % 3
				} else if m.settingCursor == 6 {
					m.selectingStep = 8
					m.textInput.Reset()
					m.textInput.SetValue(fmt.Sprintf("%s, %d", m.fontFamily, m.fontSize))
					m.textInput.Focus()
					return m, nil
				} else if m.settingCursor == 7 {
					m.selectingStep = 5
					m.textInput.Reset()
					m.textInput.SetValue(m.logDirPath)
					m.textInput.Focus()
					return m, nil
				}
				m.persistConfig()
			}
			return m, nil
		}

		if m.selectingStep == 4 {
			if m.hlInputMode {
				switch msg.String() {
				case "esc":
					m.hlInputMode = false
					m.textInput.Blur()
				case "enter":
					val := strings.TrimSpace(m.textInput.Value())
					if m.hlInputStep == 0 && val != "" {
						m.tempKeyword = val
						m.hlInputStep = 1
						m.textInput.Reset()
						m.textInput.Placeholder = "색상 (red, green...)"
					} else if m.hlInputStep == 1 {
						if val == "" {
							val = "green"
						}
						m.highlightRules = append(m.highlightRules, HighlightRule{Keyword: m.tempKeyword, Color: val})
						m.persistConfig()
						m.hlInputMode = false
						m.textInput.Blur()
					}
				default:
					m.textInput, cmd = m.textInput.Update(msg)
					return m, cmd
				}
				return m, nil
			}
			switch msg.String() {
			case "esc", "q":
				m.selectingStep = 0
			case "up", "k":
				if m.hlCursor > 0 {
					m.hlCursor--
				}
			case "down", "j":
				if m.hlCursor < len(m.highlightRules) {
					m.hlCursor++
				}
			case "enter":
				if m.hlCursor == len(m.highlightRules) && len(m.highlightRules) < 10 {
					m.hlInputMode = true
					m.hlInputStep = 0
					m.textInput.Reset()
					m.textInput.Focus()
				} else if m.hlCursor < len(m.highlightRules) {
					m.highlightRules = append(m.highlightRules[:m.hlCursor], m.highlightRules[m.hlCursor+1:]...)
					if m.hlCursor > 0 {
						m.hlCursor--
					}
					m.persistConfig()
				}
			}
			return m, nil
		}

		if m.selectingStep == 5 || m.selectingStep == 6 || m.selectingStep == 7 || m.selectingStep == 8 {
			switch msg.String() {
			case "esc":
				m.selectingStep = 0
			case "enter":
				val := strings.TrimSpace(m.textInput.Value())
				if m.selectingStep == 5 && val != "" {
					m.logDirPath = val
					m.persistConfig()
				}
				if m.selectingStep == 6 {
					m.searchQuery = val
					m.updateViewportContent()
				}
				if m.selectingStep == 8 && val != "" {
					parts := strings.Split(val, ",")
					m.fontFamily = strings.TrimSpace(parts[0])
					if len(parts) > 1 {
						fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &m.fontSize)
					}
					m.persistConfig()
				}
				m.selectingStep = 0
			default:
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
			return m, nil
		}

		// 3. [메뉴 모드]일 때의 단축키 처리
		switch msg.String() {
		case "tab":
			if m.showSplit {
				m.activePane = 1 - m.activePane
				if m.activePane == 1 {
					m.memoArea.Focus()
				} else {
					m.memoArea.Blur()
				}
			}
		case "ctrl+s":
			if m.showSplit && m.activePane == 1 {
				_ = os.MkdirAll(m.logDirPath, 0755)
				memoFile := filepath.Join(m.logDirPath, fmt.Sprintf("muart_memo_%s.txt", time.Now().Format("20060102_150405")))
				_ = os.WriteFile(memoFile, []byte(m.memoArea.Value()), 0644)
				m.writeSysLog(fmt.Sprintf("[저장 완료] %s", memoFile))
			}
		case "ctrl+o":
			if m.showSplit && m.activePane == 1 {
				m.memoCursor = 0
				m.selectingStep = 10
			}
		case "esc", "i", "enter":
			if m.activePane == 0 {
				m.isTerminalMode = true
				m.updateViewportContent()
			}
		case "q":
			return m, tea.Quit
		case "v", "V":
			m.showSplit = !m.showSplit
			m.activePane = 0
			if m.showSplit {
				m.activePane = 1
				m.memoArea.Focus()
			}
			m.viewport.Height = m.height - 4
			m.memoArea.SetHeight(m.height - 6)
			m.updateViewportContent()
		case "?":
			m.showHelp = true
		case "s", "S":
			m.settingCursor = 0
			m.selectingStep = 3
		case "h", "H":
			m.hlCursor = 0
			m.selectingStep = 4
		case "r", "R":
			if m.port != nil {
				_ = m.port.Close()
				m.port = nil
			}
			m.portGeneration++
			p, err := serial.Open(m.portName, &serial.Mode{BaudRate: m.baudRate, Parity: serial.NoParity, DataBits: 8, StopBits: serial.OneStopBit})
			if err == nil {
				m.port = p
				m.isTerminalMode = true
				return m, readSerial(m.port, m.portGeneration)
			}
		case "/":
			m.selectingStep = 6
			m.textInput.Reset()
			m.textInput.Focus()
		case "c", "C":
			m.displayLogLines = []string{""}
			m.newLineStarted = true
			m.updateViewportContent()
		case "p", "P":
			m.availablePorts, _ = serial.GetPortsList()
			m.portCursor = 0
			m.selectingStep = 1
		case "up", "k":
			m.viewport.LineUp(1)
			m.autoScroll = false
		case "down", "j":
			m.viewport.LineDown(1)
			if m.viewport.AtBottom() {
				m.autoScroll = true
			}
		}

	case serialMsg:
		if msg.generation != m.portGeneration {
			return m, nil
		}
		if msg.err != nil {
			if m.port != nil {
				_ = m.port.Close()
				m.port = nil
			}
			m.portGeneration++
			m.writeSysLog(fmt.Sprintf("\n[연결 끊김] 장치 연결 해제: %v", msg.err))
			m.isTerminalMode = false
			return m, nil
		}
		if len(msg.data) > 0 {
			m.rxBytes += int64(len(msg.data))
			m.rxPkts++
			m.processIncomingBytes(msg.data)
		}
		if m.port != nil {
			return m, readSerial(m.port, m.portGeneration)
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		boxWidth := m.width - 4
		if m.showSplit {
			m.viewport.Width = (boxWidth / 2) - 3
			m.memoArea.SetWidth(boxWidth - m.viewport.Width - 4)
			m.memoArea.SetHeight(m.height - 6)
		} else {
			m.viewport.Width = boxWidth - 2
		}
		m.viewport.Height = m.height - 4
	}

	m.viewport, _ = m.viewport.Update(msg)
	if m.showSplit && m.activePane == 1 {
		m.memoArea, cmd = m.memoArea.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) View() string {
	ts := getThemeStyles(m.theme)

	if m.showHelp {
		h := `muart Terminal (리얼 터미널 에뮬레이터)

[기본 조작 (터미널 모드)]
  모든 키보드 입력(Enter, Tab, 방향키, Ctrl+C 등)이 장비로 즉시 전송됩니다.
  Esc            터미널 제어를 잠시 멈추고 [메뉴 모드]로 진입합니다.

[메뉴 모드 전용 단축키] (Esc를 누른 상태에서 사용)
  Esc, Enter, i  다시 터미널 복귀
  p              시리얼 포트 선택
  r              포트 재연결
  s              설정 메뉴
  v              화면 분할 / 메모장 켜기 (Tab으로 포커스 전환)
  c              화면 로그 지우기
  /              로그 검색
  q              프로그램 완전 종료

아무 키나 눌러 닫기`
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Width(66).Render(h))
	}

	// [누락되었던 설정 메뉴 팝업 렌더링 추가]
	if m.selectingStep == 3 {
		s := "환경 설정 (Settings)\n\n"
		settingsList := []string{
			fmt.Sprintf("타임스탬프 (Timestamp): %v", m.showTimestamp),
			fmt.Sprintf("오토스크롤 (Auto Scroll): %v", m.autoScroll),
			fmt.Sprintf("파일 로깅 (File Logging): %v", m.isLogging),
			fmt.Sprintf("헥스 모드 (Hex Mode): %v", m.isHexMode),
			fmt.Sprintf("줄바꿈 (Line Ending): %s", m.lineEnding),
			fmt.Sprintf("테마 (Theme): %d", m.theme),
			fmt.Sprintf("폰트 설정: %s, %d", m.fontFamily, m.fontSize),
			fmt.Sprintf("저장 경로: %s", m.logDirPath),
		}
		for i, label := range settingsList {
			if i == m.settingCursor {
				s += "> " + lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Render(label) + "\n"
			} else {
				s += "  " + label + "\n"
			}
		}
		s += "\n(상하 이동: 방향키/j/k | 값 변경: Enter | 닫기: Esc)"
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Width(60).Render(s))
	}

	if m.selectingStep >= 4 && m.selectingStep <= 8 {
		var title string
		if m.selectingStep == 4 {
			title = "키워드 추가:"
		}
		if m.selectingStep == 5 {
			title = "저장 폴더 경로:"
		}
		if m.selectingStep == 6 {
			title = "검색어 입력:"
		}
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Render(title+"\n\n"+m.textInput.View()))
	}

	if m.selectingStep == 1 {
		s := "포트 선택\n\n"
		for i, p := range m.availablePorts {
			if i == m.portCursor {
				s += "> " + lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Render(p) + "\n"
			} else {
				s += "  " + p + "\n"
			}
		}
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Render(s))
	}
	if m.selectingStep == 2 {
		s := "보드레이트 선택\n\n"
		for i, b := range m.baudRates {
			if i == m.baudCursor {
				s += fmt.Sprintf("> %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Render(fmt.Sprint(b)))
			} else {
				s += fmt.Sprintf("  %d\n", b)
			}
		}
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Render(s))
	}
	if m.selectingStep == 9 {
		s := "줄바꿈(Line Ending)\n\n"
		for i, le := range m.lineEndings {
			if i == m.lineEndCursor {
				s += fmt.Sprintf("> %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Render(le))
			} else {
				s += fmt.Sprintf("  %s\n", le)
			}
		}
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Render(s))
	}

	title := ts.Title.Render("muart Real Terminal")
	statusStr := "CONNECTED"
	if m.port == nil {
		statusStr = "DISCONNECTED"
	}
	stColor := lipgloss.Color("46")
	if m.port == nil {
		stColor = lipgloss.Color("196")
	}
	statusStyled := lipgloss.NewStyle().Foreground(stColor).Bold(true).Render(statusStr)

	modeStr := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Background(lipgloss.Color("235")).Bold(true).Render(" MENU MODE (Press Esc to terminal) ")
	if m.isTerminalMode {
		modeStr = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("22")).Bold(true).Render(" TERMINAL MODE (Esc for menu) ")
	}

	footerText := fmt.Sprintf(" 상태: %s | 포트: %s | Baud: %d | RX: %dB | TX: %dB | %s ", statusStyled, m.portName, m.baudRate, m.rxBytes, m.txBytes, modeStr)
	boxWidth := m.width - 4
	if boxWidth < 30 {
		boxWidth = 30
	}
	footerBox := ts.Footer.Width(boxWidth).Render(footerText)

	var mainView string
	if m.showSplit {
		leftBox := ts.Box.Width(m.viewport.Width).Render(m.viewport.View())
		rightBox := ts.Box.Width(m.memoArea.Width()).Render(m.memoArea.View())
		mainView = lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)
	} else {
		mainView = ts.Box.Width(boxWidth).Render(m.viewport.View())
	}

	contentArea := lipgloss.JoinVertical(lipgloss.Left, title, mainView, footerBox)
	return ts.AppBorder.Width(m.width - 2).Height(m.height - 2).Render(contentArea)
}

func main() {
	cfg := loadConfig()
	port, _ := serial.Open(cfg.PortName, &serial.Mode{BaudRate: cfg.BaudRate, Parity: serial.NoParity, DataBits: 8, StopBits: serial.OneStopBit})

	ti := textinput.New()
	ti.Width = 40
	ma := textarea.New()
	ma.SetValue("메모장 (Ctrl+S 저장)")
	vp := viewport.New(80, 18)

	intro := []string{"포트 설정이나 메뉴 조작이 필요하면 [Esc]를 누르세요.", ""}

	m := model{
		port: port, portName: cfg.PortName, baudRate: cfg.BaudRate, lineEnding: cfg.LineEnding,
		viewport: vp, memoArea: ma, textInput: ti,
		displayLogLines: intro, newLineStarted: true,
		baudRates:  []int{9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600},
		baudCursor: 4, lineEndings: []string{"없음", "LF", "CR", "CRLF"}, lineEndCursor: 3,
		isTerminalMode: true,
		theme:          cfg.Theme, fontFamily: cfg.FontFamily, fontSize: cfg.FontSize,
		highlightRules: cfg.HighlightRules,
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		log.Fatalf("오류: %v", err)
	}
}
