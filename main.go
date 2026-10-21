package main

import (
	"encoding/hex"
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

// UI 메모리 방어를 위한 화면 표시 버퍼 제한
const (
	maxDisplayLogLines   = 5000
	maxDisplayLogBytes   = 2 * 1024 * 1024
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
		ShowTimestamp:  true,
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
	if cfg.LineEnding == "" {
		cfg.LineEnding = defaultCfg.LineEnding
	}
	if cfg.BaudRate <= 0 {
		cfg.BaudRate = defaultCfg.BaudRate
	}
	if cfg.FontSize < 6 || cfg.FontSize > 48 {
		cfg.FontSize = defaultCfg.FontSize
	}
	if cfg.Theme < 0 || cfg.Theme > 2 {
		cfg.Theme = defaultCfg.Theme
	}
	if cfg.LogDirPath == "" {
		cfg.LogDirPath = defaultCfg.LogDirPath
	}
	if cfg.HighlightRules == nil {
		cfg.HighlightRules = []HighlightRule{}
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
	if err := os.Rename(tmp, configFile); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (m *model) persistConfig() {
	if err := m.saveConfig(); err != nil {
		m.writeLog(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("[에러] 설정 파일 저장 실패 (%s): %v", configFile, err)))
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
	case 1: // Ocean Blue
		return ThemeStyles{
			AppBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("33")),
			Box:       lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("51")).Padding(0, 1),
			Footer:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("75")).Padding(0, 1),
			Title:     lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true).Italic(true),
			Help:      lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
			Modal:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39")).Padding(1, 3).Width(55),
		}
	case 2: // Amber Retro
		return ThemeStyles{
			AppBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("208")),
			Box:       lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("214")).Padding(0, 1),
			Footer:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("202")).Padding(0, 1),
			Title:     lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Italic(true),
			Help:      lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
			Modal:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("208")).Padding(1, 3).Width(55),
		}
	default: // 0: Cyberpunk (Default)
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

	showSplit       bool
	showHelp        bool
	logBuffer       string
	fullLogBuffer   string
	displayLogLines []string
	displayLogHead  int
	displayLogBytes int
	searchQuery     string
	width           int
	height          int

	cmdHistory   []string
	historyIndex int

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

	showTimestamp bool
	autoScroll    bool
	isLogging     bool
	isHexMode     bool
	theme         int
	fontFamily    string
	fontSize      int
	logDirPath    string
	logFile       *os.File
	inInputMode   bool
	settingCursor int

	highlightRules []HighlightRule
	hlCursor       int
	hlInputMode    bool
	hlInputStep    int
	tempKeyword    string

	rxLineBuffer string
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
	case "magenta":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("201")).Bold(true)
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
	if strings.Contains(line, "[HEX]") {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Italic(true).Render(line)
	}
	return line
}

func (m *model) formatLogData(raw string) (string, string) {
	var fileLine string
	var displayLine string

	if m.showTimestamp {
		ts := time.Now().Format("15:04:05.000")
		fileLine = fmt.Sprintf("[%s] %s", ts, raw)
	} else {
		fileLine = raw
	}

	displayLine = fileLine

	if m.isHexMode && !strings.HasPrefix(raw, "[설정]") && !strings.HasPrefix(raw, "[에러]") && !strings.HasPrefix(raw, "[연결 끊김]") && !strings.HasPrefix(raw, "[로그") && !strings.HasPrefix(raw, "[RX ") {
		hexStr := hex.EncodeToString([]byte(raw))
		formattedHex := ""
		for i := 0; i < len(hexStr); i += 2 {
			end := i + 2
			if end > len(hexStr) {
				end = len(hexStr)
			}
			formattedHex += hexStr[i:end] + " "
		}
		formattedHex = strings.TrimSpace(formattedHex)
		displayLine += fmt.Sprintf(" [HEX: %s]", formattedHex)
		fileLine += fmt.Sprintf(" [HEX: %s]", formattedHex)
	}

	return displayLine, fileLine
}

func (m *model) appendDisplayLine(line string) {
	m.displayLogLines = append(m.displayLogLines, line)
	m.displayLogBytes += len(line) + 1
	for len(m.displayLogLines)-m.displayLogHead > maxDisplayLogLines || m.displayLogBytes > maxDisplayLogBytes {
		if m.displayLogHead >= len(m.displayLogLines) {
			break
		}
		m.displayLogBytes -= len(m.displayLogLines[m.displayLogHead]) + 1
		m.displayLogLines[m.displayLogHead] = ""
		m.displayLogHead++
	}
	// 메모리 해제를 위한 주기적 압축
	if m.displayLogHead > 1024 && m.displayLogHead*2 >= len(m.displayLogLines) {
		remaining := copy(m.displayLogLines, m.displayLogLines[m.displayLogHead:])
		for i := remaining; i < len(m.displayLogLines); i++ {
			m.displayLogLines[i] = ""
		}
		m.displayLogLines = m.displayLogLines[:remaining]
		m.displayLogHead = 0
	}
}

func (m *model) updateViewportContent() {
	m.fullLogBuffer = strings.Join(m.displayLogLines[m.displayLogHead:], "\n")
	content := m.fullLogBuffer
	if m.searchQuery != "" {
		lines := strings.Split(m.fullLogBuffer, "\n")
		filtered := make([]string, 0)
		needle := strings.ToUpper(m.searchQuery)
		for _, line := range lines {
			if strings.Contains(strings.ToUpper(line), needle) {
				filtered = append(filtered, line)
			}
		}
		if len(filtered) == 0 {
			content = fmt.Sprintf("[검색 결과 없음: %q]", m.searchQuery)
		} else {
			content = strings.Join(filtered, "\n")
		}
	}
	m.viewport.SetContent(content + "\n" + m.textInput.View())
	if m.autoScroll {
		m.viewport.GotoBottom()
	}
}

func (m *model) writeFileLog(line string) {
	if !m.isLogging || m.logFile == nil {
		return
	}
	if _, err := m.logFile.WriteString(line + "\n"); err != nil {
		_ = m.logFile.Close()
		m.logFile = nil
		m.isLogging = false
		stamp := time.Now().Format("15:04:05.000")
		// 파일 로깅 실패 시 강렬한 에러 표시
		errMsg := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(fmt.Sprintf("[%s] [에러] 파일 쓰기 실패로 로깅을 중지합니다: %v", stamp, err))
		m.appendDisplayLine(errMsg)
	}
}

func (m *model) appendLogEntry(rawText string) {
	displayLine, fileLine := m.formatLogData(rawText)
	m.appendDisplayLine(highlightLog(displayLine, m.highlightRules))
	m.writeFileLog(fileLine)
}

func (m *model) writeLog(rawText string) {
	m.appendLogEntry(rawText)
	m.updateViewportContent()
}

func (m *model) writeLogBatch(lines []string) {
	for _, line := range lines {
		if line != "" {
			m.appendLogEntry(line)
		}
	}
	m.updateViewportContent()
}

func (m *model) writeTxLog(rawText string) {
	displayLine, fileLine := m.formatLogData(rawText)
	m.appendDisplayLine(lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render(displayLine))
	m.writeFileLog(fileLine)
	m.updateViewportContent()
}

func printableASCII(data []byte) string {
	var b strings.Builder
	b.Grow(len(data))
	for _, c := range data {
		if c >= 0x20 && c <= 0x7e {
			b.WriteByte(c)
		} else {
			b.WriteByte('.')
		}
	}
	return b.String()
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
	// 마우스 휠 스크롤 지원 (터미널 뷰포트 탐색 편의성 극대화)
	case tea.MouseMsg:
		if m.selectingStep == 0 {
			switch msg.Type {
			case tea.MouseWheelUp:
				m.viewport.LineUp(3)
				m.autoScroll = false // 수동 스크롤 시 오토스크롤 임시 정지
				return m, nil
			case tea.MouseWheelDown:
				m.viewport.LineDown(3)
				if m.viewport.AtBottom() {
					m.autoScroll = true // 끝까지 내리면 오토스크롤 복귀
				}
				return m, nil
			}
		}

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			if m.port != nil {
				_ = m.port.Close()
			}
			if m.logFile != nil {
				_ = m.logFile.Close()
			}
			return m, tea.Quit
		}

		if m.showHelp {
			switch msg.String() {
			case "?", "esc", "q":
				m.showHelp = false
			}
			return m, nil
		}

		if msg.String() == "?" && !m.inInputMode && m.selectingStep == 0 && !m.hlInputMode {
			m.showHelp = true
			return m, nil
		}

		// (생략 없이 기존 모달창 및 스텝 처리 로직 유지)
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
					if err != nil {
						m.writeLog(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("[에러] 메모 파일 읽기 실패: %v", err)))
					} else {
						m.memoArea.SetValue(string(data))
						m.writeLog(fmt.Sprintf("[메모] 파일 불러오기 성공: %s", selectedFile))
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
				m.textInput.Reset()
				if m.inInputMode {
					m.textInput.Focus()
				}
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
					m.textInput.Reset()
					if m.inInputMode {
						m.textInput.Focus()
					}
				}
			}
			return m, nil
		}

		if m.selectingStep == 2 {
			switch msg.String() {
			case "esc", "q":
				m.selectingStep = 0
				m.textInput.Reset()
				if m.inInputMode {
					m.textInput.Focus()
				}
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
				m.textInput.Reset()
				if m.inInputMode {
					m.textInput.Focus()
				}
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
				m.rxLineBuffer = ""
				m.portName = m.tempPortName
				m.baudRate = selectedBaud
				m.lineEnding = selectedLineEnding

				mode := &serial.Mode{
					BaudRate: m.baudRate,
					Parity:   serial.NoParity,
					DataBits: 8,
					StopBits: serial.OneStopBit,
				}
				p, err := serial.Open(m.portName, mode)
				if err != nil {
					m.port = nil
					m.writeLog(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("[에러] 포트 열기 실패 (%s): %v", m.portName, err)))
				} else {
					m.port = p
					m.writeLog(fmt.Sprintf("[설정 변경] 포트: %s, 보드레이트: %d, 줄끝: %s 연결 성공", m.portName, m.baudRate, m.lineEnding))
				}

				m.persistConfig()

				m.selectingStep = 0
				m.textInput.Reset()
				if m.inInputMode {
					m.textInput.Focus()
				}
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
				m.textInput.Reset()
				if m.inInputMode {
					m.textInput.Focus()
				}
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
					statusStr := "OFF [x]"
					if m.showTimestamp {
						statusStr = "ON [✓]"
					}
					m.writeLog(fmt.Sprintf("[설정] 타임스탬프 기능: %s", statusStr))
				} else if m.settingCursor == 1 {
					m.autoScroll = !m.autoScroll
					statusStr := "OFF [x]"
					if m.autoScroll {
						statusStr = "ON [✓]"
						m.viewport.GotoBottom()
					}
					m.writeLog(fmt.Sprintf("[설정] 오토스크롤 기능: %s", statusStr))
				} else if m.settingCursor == 2 {
					if m.isLogging {
						if m.logFile != nil {
							_ = m.logFile.Close()
							m.logFile = nil
						}
						m.isLogging = false
						m.writeLog("[설정] 파일 로깅 기능: OFF [x]")
					} else {
						if err := os.MkdirAll(m.logDirPath, 0755); err != nil {
							m.writeLog(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("[에러] 로그 폴더 생성 실패 (%s): %v", m.logDirPath, err)))
							return m, nil
						}

						filename := filepath.Join(m.logDirPath, fmt.Sprintf("muart_log_%s.txt", time.Now().Format("20060102_150405")))
						f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
						if err != nil {
							m.writeLog(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("[에러] 로그 파일 생성 실패: %v", err)))
						} else {
							m.logFile = f
							m.isLogging = true
							m.writeLog(fmt.Sprintf("[설정] 파일 로깅 기능: ON [✓] (저장 파일: %s)", filename))
						}
					}
				} else if m.settingCursor == 3 {
					m.isHexMode = !m.isHexMode
					hexStatus := "OFF [x]"
					if m.isHexMode {
						hexStatus = "ON [✓]"
					}
					m.writeLog(fmt.Sprintf("[설정] Hex 뷰어 모드: %s", hexStatus))
				} else if m.settingCursor == 4 {
					for i, le := range m.lineEndings {
						if le == m.lineEnding {
							m.lineEnding = m.lineEndings[(i+1)%len(m.lineEndings)]
							break
						}
					}
					m.writeLog(fmt.Sprintf("[설정] 줄 끝 처리 변경: %s", m.lineEnding))
				} else if m.settingCursor == 5 {
					m.theme = (m.theme + 1) % 3
					themeName := "Cyberpunk"
					if m.theme == 1 {
						themeName = "Ocean Blue"
					} else if m.theme == 2 {
						themeName = "Amber Retro"
					}
					m.writeLog(fmt.Sprintf("[설정] 테마 변경: %s", themeName))
				} else if m.settingCursor == 6 {
					m.selectingStep = 8
					m.textInput.Reset()
					m.textInput.Placeholder = "예: JetBrains Mono, 14"
					m.textInput.SetValue(fmt.Sprintf("%s, %d", m.fontFamily, m.fontSize))
					m.textInput.Focus()
					return m, nil
				} else if m.settingCursor == 7 {
					m.selectingStep = 5
					m.textInput.Reset()
					m.textInput.Placeholder = "예: ./logs 또는 /Users/name/Desktop"
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
					m.textInput.Reset()
					m.textInput.Blur()
				case "enter":
					val := strings.TrimSpace(m.textInput.Value())
					if m.hlInputStep == 0 {
						if val != "" {
							m.tempKeyword = val
							m.hlInputStep = 1
							m.textInput.SetValue("")
							m.textInput.Placeholder = "색상 입력 (red, green, yellow, blue, cyan)"
						}
					} else {
						if val == "" {
							val = "green"
						}
						if len(m.highlightRules) < 10 {
							m.highlightRules = append(m.highlightRules, HighlightRule{
								Keyword: m.tempKeyword,
								Color:   val,
							})
							m.persistConfig()
						}
						m.hlInputMode = false
						m.textInput.Reset()
						m.textInput.Blur()
					}
				default:
					var c tea.Cmd
					m.textInput, c = m.textInput.Update(msg)
					return m, c
				}
				return m, nil
			}

			switch msg.String() {
			case "esc", "q":
				m.selectingStep = 0
				m.textInput.Reset()
				if m.inInputMode {
					m.textInput.Focus()
				}
			case "up", "k":
				if m.hlCursor > 0 {
					m.hlCursor--
				}
			case "down", "j":
				maxCursor := len(m.highlightRules)
				if len(m.highlightRules) < 10 {
					maxCursor = len(m.highlightRules)
				}
				if m.hlCursor < maxCursor {
					m.hlCursor++
				}
			case "enter":
				maxIdx := len(m.highlightRules)
				if m.hlCursor == maxIdx && len(m.highlightRules) < 10 {
					m.hlInputMode = true
					m.hlInputStep = 0
					m.tempKeyword = ""
					m.textInput.Reset()
					m.textInput.Placeholder = "추가할 키워드 입력 (예: ERROR, OK)"
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

		if m.selectingStep == 5 {
			switch msg.String() {
			case "esc":
				m.selectingStep = 3
				m.textInput.Reset()
				if m.inInputMode {
					m.textInput.Focus()
				}
			case "enter":
				newPath := strings.TrimSpace(m.textInput.Value())
				if newPath != "" {
					m.logDirPath = newPath
					m.persistConfig()
					if m.isLogging {
						if m.logFile != nil {
							_ = m.logFile.Close()
							m.logFile = nil
						}
						if err := os.MkdirAll(m.logDirPath, 0755); err == nil {
							filename := filepath.Join(m.logDirPath, fmt.Sprintf("muart_log_%s.txt", time.Now().Format("20060102_150405")))
							if f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
								m.logFile = f
								m.writeLog(fmt.Sprintf("[설정] 로그 저장 폴더 변경 및 파일 재연결 완료: %s", filename))
							} else {
								m.isLogging = false
								m.writeLog(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("[에러] 새 경로 파일 생성 실패: %v", err)))
							}
						} else {
							m.isLogging = false
							m.writeLog(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("[에러] 폴더 생성 실패: %v", err)))
						}
					} else {
						m.writeLog(fmt.Sprintf("[설정] 로그 저장 폴더 경로가 변경되었습니다: %s", m.logDirPath))
					}
				}
				m.selectingStep = 3
				m.textInput.Reset()
				if m.inInputMode {
					m.textInput.Focus()
				}
			default:
				var c tea.Cmd
				m.textInput, c = m.textInput.Update(msg)
				return m, c
			}
			return m, nil
		}

		if m.selectingStep == 6 {
			switch msg.String() {
			case "esc":
				m.selectingStep = 0
				m.textInput.Reset()
				if m.inInputMode {
					m.textInput.Focus()
				}
			case "enter":
				m.searchQuery = strings.TrimSpace(m.textInput.Value())
				m.updateViewportContent()
				m.selectingStep = 0
				m.textInput.Reset()
				if m.inInputMode {
					m.textInput.Focus()
				}
			default:
				var c tea.Cmd
				m.textInput, c = m.textInput.Update(msg)
				return m, c
			}
			return m, nil
		}

		if m.selectingStep == 7 {
			switch msg.String() {
			case "esc":
				m.selectingStep = 0
				m.textInput.Reset()
				if m.inInputMode {
					m.textInput.Focus()
				}
			case "enter":
				exportFilename := strings.TrimSpace(m.textInput.Value())
				if exportFilename == "" {
					exportFilename = fmt.Sprintf("muart_export_%s.txt", time.Now().Format("20060102_150405"))
				}
				if !strings.HasSuffix(exportFilename, ".txt") {
					exportFilename += ".txt"
				}

				fullExportPath := filepath.Join(m.logDirPath, exportFilename)
				if err := os.MkdirAll(m.logDirPath, 0755); err == nil {
					err = os.WriteFile(fullExportPath, []byte(m.fullLogBuffer), 0644)
					if err != nil {
						m.writeLog(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("[에러] 로그 내보내기 실패: %v", err)))
					} else {
						m.writeLog(fmt.Sprintf("[설정] 로그가 성공적으로 내보내기 되었습니다: %s", fullExportPath))
					}
				} else {
					m.writeLog(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("[에러] 내보내기 폴더 생성 실패: %v", err)))
				}

				m.selectingStep = 0
				m.textInput.Reset()
				if m.inInputMode {
					m.textInput.Focus()
				}
			default:
				var c tea.Cmd
				m.textInput, c = m.textInput.Update(msg)
				return m, c
			}
			return m, nil
		}

		if m.selectingStep == 8 {
			switch msg.String() {
			case "esc":
				m.selectingStep = 3
				m.textInput.Reset()
				if m.inInputMode {
					m.textInput.Focus()
				}
			case "enter":
				inputVal := strings.TrimSpace(m.textInput.Value())
				if inputVal != "" {
					parts := strings.Split(inputVal, ",")
					family := strings.TrimSpace(parts[0])
					size := 14
					if len(parts) > 1 {
						var parsedSize int
						_, err := fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &parsedSize)
						if err == nil && parsedSize > 0 {
							size = parsedSize
						}
					}
					m.fontFamily = family
					m.fontSize = size
					m.persistConfig()
					m.writeLog(fmt.Sprintf("[설정] 폰트 변경 완료: 패밀리 [%s], 크기 [%dpt]", m.fontFamily, m.fontSize))
				}
				m.selectingStep = 3
				m.textInput.Reset()
				if m.inInputMode {
					m.textInput.Focus()
				}
			default:
				var c tea.Cmd
				m.textInput, c = m.textInput.Update(msg)
				return m, c
			}
			return m, nil
		}

		switch msg.String() {
		case "tab":
			if m.showSplit {
				m.activePane = 1 - m.activePane
				if m.activePane == 0 {
					m.memoArea.Blur()
					if m.inInputMode {
						m.textInput.Focus()
					}
				} else {
					m.textInput.Blur()
					m.memoArea.Focus()
				}
			}
			return m, nil

		case "ctrl+s":
			if m.showSplit && m.activePane == 1 {
				memoFilename := fmt.Sprintf("muart_memo_%s.txt", time.Now().Format("20060102_150405"))
				fullMemoPath := filepath.Join(m.logDirPath, memoFilename)
				if err := os.MkdirAll(m.logDirPath, 0755); err == nil {
					err = os.WriteFile(fullMemoPath, []byte(m.memoArea.Value()), 0644)
					if err != nil {
						m.writeLog(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("[에러] 메모 저장 실패: %v", err)))
					} else {
						m.writeLog(fmt.Sprintf("[메모] 메모가 성공적으로 저장되었습니다: %s", fullMemoPath))
					}
				} else {
					m.writeLog(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("[에러] 메모 폴더 생성 실패: %v", err)))
				}
			}
			return m, nil

		case "ctrl+o":
			if m.showSplit && m.activePane == 1 {
				files, err := os.ReadDir(m.logDirPath)
				m.memoFiles = []string{}
				if err == nil {
					for _, f := range files {
						if !f.IsDir() && strings.HasPrefix(f.Name(), "muart_memo_") && strings.HasSuffix(f.Name(), ".txt") {
							m.memoFiles = append(m.memoFiles, f.Name())
						}
					}
				}
				m.memoCursor = 0
				m.selectingStep = 10
			}
			return m, nil

		case "esc":
			if m.activePane == 0 {
				m.inInputMode = !m.inInputMode
				if m.inInputMode {
					m.textInput.Focus()
					m.textInput.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).Render("> ")
				} else {
					m.textInput.Blur()
					m.textInput.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true).Render("(cmd)> ")
				}
				m.updateViewportContent()
			}
			return m, nil

		case "v", "V":
			m.showSplit = !m.showSplit

			appWidth := m.width
			if appWidth <= 0 {
				appWidth = 80
			}
			boxWidth := appWidth - 4
			if boxWidth < 30 {
				boxWidth = 30
			}

			if m.showSplit {
				halfWidth := (boxWidth / 2) - 1
				rightWidth := boxWidth - halfWidth - 2
				m.viewport.Width = halfWidth - 4
				m.memoArea.SetWidth(rightWidth - 6)
				m.activePane = 1
				m.textInput.Blur()
				m.memoArea.Focus()
			} else {
				m.viewport.Width = boxWidth - 2
				m.activePane = 0
				m.memoArea.Blur()
				if m.inInputMode {
					m.textInput.Focus()
				}
			}
			m.viewport.Height = m.height - 10
			m.memoArea.SetHeight(m.height - 13)
			m.textInput.Width = boxWidth - 6

			m.updateViewportContent()
			return m, nil
		}

		if m.showSplit && m.activePane == 1 {
			m.memoArea, cmd = m.memoArea.Update(msg)
			return m, cmd
		}

		if m.inInputMode {
			switch msg.String() {
			case "enter":
				typedText := m.textInput.Value()
				if typedText != "" {
					m.cmdHistory = append(m.cmdHistory, typedText)
					m.historyIndex = -1

					var payload []byte
					isHexTx := false

					// Hex 바이트 직접 송신 기능 지원 ("hex: DE AD BE EF")
					if strings.HasPrefix(strings.ToLower(typedText), "hex:") {
						isHexTx = true
						hexStr := strings.TrimSpace(typedText[4:])
						hexStr = strings.ReplaceAll(hexStr, " ", "")
						decoded, err := hex.DecodeString(hexStr)
						if err != nil {
							m.writeLog(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("[송신 에러] 잘못된 Hex 형식입니다: %v", err)))
							m.textInput.Reset()
							return m, nil
						}
						payload = decoded
					} else {
						suffix := ""
						switch m.lineEnding {
						case "LF":
							suffix = "\n"
						case "CR":
							suffix = "\r"
						case "CRLF":
							suffix = "\r\n"
						case "없음":
							suffix = ""
						}
						payload = []byte(typedText + suffix)
					}

					m.txBytes += int64(len(payload))
					m.txPkts++

					if m.port != nil {
						_, err := m.port.Write(payload)
						if err != nil {
							m.writeLog(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(fmt.Sprintf("[송신 FAIL] 연결 문제 발생: %v", err)))
							_ = m.port.Close()
							m.port = nil
						} else {
							if isHexTx {
								m.writeTxLog(fmt.Sprintf("[TX HEX] % X", payload))
							} else {
								m.writeTxLog(typedText)
							}
						}
					} else {
						m.writeLog(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render("[에러] 연결된 시리얼 포트가 없습니다. 'Esc'를 눌러 모드 전환 후 'p'를 눌러 포트를 선택하세요."))
					}
					m.textInput.Reset()
				}
			case "up":
				if len(m.cmdHistory) > 0 {
					if m.historyIndex == -1 {
						m.historyIndex = len(m.cmdHistory) - 1
					} else if m.historyIndex > 0 {
						m.historyIndex--
					}
					m.textInput.SetValue(m.cmdHistory[m.historyIndex])
					m.textInput.CursorEnd()
				}
				m.updateViewportContent()
				return m, nil
			case "down":
				if len(m.cmdHistory) > 0 && m.historyIndex != -1 {
					if m.historyIndex < len(m.cmdHistory)-1 {
						m.historyIndex++
						m.textInput.SetValue(m.cmdHistory[m.historyIndex])
					} else {
						m.historyIndex = -1
						m.textInput.SetValue("")
					}
					m.textInput.CursorEnd()
				}
				m.updateViewportContent()
				return m, nil
			default:
				m.textInput, cmd = m.textInput.Update(msg)
				m.updateViewportContent()
				return m, cmd
			}
		} else {
			switch msg.String() {
			case "s", "S":
				m.textInput.Blur()
				m.settingCursor = 0
				m.selectingStep = 3
				return m, nil
			case "h", "H":
				m.textInput.Blur()
				m.hlCursor = 0
				m.selectingStep = 4
				return m, nil
			case "r", "R":
				if m.port != nil {
					_ = m.port.Close()
					m.port = nil
				}
				if m.portName == "" {
					m.writeLog(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("[에러] 설정된 포트 이름이 없습니다. 'p'를 눌러 포트를 선택하세요."))
					return m, nil
				}
				mode := &serial.Mode{
					BaudRate: m.baudRate,
					Parity:   serial.NoParity,
					DataBits: 8,
					StopBits: serial.OneStopBit,
				}
				m.portGeneration++
				m.rxLineBuffer = ""
				p, err := serial.Open(m.portName, mode)
				if err != nil {
					m.port = nil
					m.writeLog(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("[에러] 재연결 실패 (%s): %v", m.portName, err)))
					return m, nil
				} else {
					m.port = p
					m.writeLog(fmt.Sprintf("[설정] 시리얼 포트 재연결 성공: %s (%d)", m.portName, m.baudRate))
					return m, readSerial(m.port, m.portGeneration)
				}
			case "e", "E":
				m.textInput.Blur()
				m.selectingStep = 7
				m.textInput.Reset()
				m.textInput.Placeholder = "예: export_result.txt"
				m.textInput.SetValue(fmt.Sprintf("muart_export_%s.txt", time.Now().Format("20060102_150405")))
				m.textInput.Focus()
				return m, nil
			case "/":
				m.textInput.Blur()
				m.selectingStep = 6
				m.textInput.Reset()
				m.textInput.Placeholder = "검색할 키워드 입력 (비우고 엔터: 필터 해제)"
				m.textInput.SetValue(m.searchQuery)
				m.textInput.Focus()
				return m, nil
			case "c", "C":
				displayLine, _ := m.formatLogData("[로그가 초기화되었습니다]")
				cleared := highlightLog(displayLine, m.highlightRules)
				m.displayLogLines = []string{cleared}
				m.displayLogHead = 0
				m.displayLogBytes = len(cleared)
				m.updateViewportContent()
				m.viewport.GotoTop()
			case "p":
				m.textInput.Blur()
				ports, err := serial.GetPortsList()
				if err != nil {
					m.availablePorts = []string{}
				} else {
					m.availablePorts = ports
				}
				m.portCursor = 0
				m.selectingStep = 1
				return m, nil
			case "up", "k":
				m.viewport.LineUp(1)
				m.autoScroll = false
			case "down", "j":
				m.viewport.LineDown(1)
				if m.viewport.AtBottom() {
					m.autoScroll = true
				}
			}
		}

	case serialMsg:
		if msg.generation != m.portGeneration {
			return m, nil
		}
		// 포트 연결 끊김(물리적 탈착 등)에 대한 강력한 예외 처리
		if msg.err != nil {
			if m.port != nil {
				_ = m.port.Close()
				m.port = nil
			}
			m.portGeneration++
			m.rxLineBuffer = ""
			errLog := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(fmt.Sprintf("[연결 끊김] 장치와 통신할 수 없습니다: %v", msg.err))
			m.writeLog(errLog)
			return m, nil
		}
		if len(msg.data) > 0 {
			m.rxBytes += int64(len(msg.data))
			m.rxPkts++
			if m.isHexMode {
				m.writeLog(fmt.Sprintf("[RX %dB] HEX: % X | ASCII: %s", len(msg.data), msg.data, printableASCII(msg.data)))
			} else {
				m.rxLineBuffer += string(msg.data)
				lines := make([]string, 0, 8)
				for {
					idx := strings.IndexAny(m.rxLineBuffer, "\r\n")
					if idx < 0 {
						break
					}
					line := m.rxLineBuffer[:idx]
					delim := 1
					if m.rxLineBuffer[idx] == '\r' && idx+1 < len(m.rxLineBuffer) && m.rxLineBuffer[idx+1] == '\n' {
						delim = 2
					}
					m.rxLineBuffer = m.rxLineBuffer[idx+delim:]
					if line != "" {
						lines = append(lines, line)
					}
				}
				if len(m.rxLineBuffer) > 64*1024 {
					lines = append(lines, "[경고] 줄바꿈 없는 수신 데이터가 64KiB를 초과해 조각으로 표시합니다.", m.rxLineBuffer)
					m.rxLineBuffer = ""
				}
				if len(lines) > 0 {
					m.writeLogBatch(lines)
				}
			}
		}
		if m.port != nil {
			return m, readSerial(m.port, m.portGeneration)
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		appWidth := m.width
		if appWidth <= 0 {
			appWidth = 80
		}

		boxWidth := appWidth - 4
		if boxWidth < 30 {
			boxWidth = 30
		}

		if m.showSplit {
			halfWidth := (boxWidth / 2) - 1
			rightWidth := boxWidth - halfWidth - 2
			m.viewport.Width = halfWidth - 4
			m.memoArea.SetWidth(rightWidth - 6)
			m.memoArea.SetHeight(m.height - 13)
		} else {
			m.viewport.Width = boxWidth - 2
		}
		m.viewport.Height = m.height - 10
		m.textInput.Width = boxWidth - 6
	}

	if m.selectingStep == 0 && m.inInputMode && m.activePane == 0 {
		m.textInput, cmd = m.textInput.Update(msg)
		m.updateViewportContent()
		m.viewport, _ = m.viewport.Update(msg)
		return m, cmd
	}

	m.viewport, _ = m.viewport.Update(msg)
	if m.showSplit {
		m.memoArea, cmd = m.memoArea.Update(msg)
		return m, cmd
	}
	return m, cmd
}

func (m model) View() string {
	ts := getThemeStyles(m.theme)

	if m.showHelp {
		modalContent := `muart Terminal — 단축키 도움말

[공통 / 로그 화면]
  ?             단축키 도움말 열기/닫기
  p             시리얼 포트 선택
  s             설정 메뉴 열기
  h             로그 하이라이트 규칙 설정
  r             현재 포트 재연결
  v             화면 분할 / 메모장 표시 전환
  Tab           로그 화면과 메모장 포커스 전환 (분할 화면)
  Esc           명령 입력 모드 ↔ 명령/탐색 모드 전환
  c             화면 로그 지우기
  /             로그 검색 / 검색 필터 해제
  e             로그 내보내기
  ↑ / k         로그 위로 이동 (마우스 스크롤 지원)
  ↓ / j         로그 아래로 이동

[명령 입력 모드]
  Enter         명령 전송 (hex: DE AD 입력 시 Raw Hex 전송)
  ↑ / ↓         이전 / 다음 명령 기록

[메모장 분할 화면]
  Ctrl+S        메모 저장
  Ctrl+O        저장된 메모 불러오기

[메뉴 / 선택 창]
  ↑ / k         이전 항목
  ↓ / j         다음 항목
  Enter         선택 / 적용
  Esc 또는 q    취소 / 닫기

Ctrl+C         프로그램 종료

? / Esc / q 를 눌러 이 창을 닫습니다.`
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Width(66).Render(modalContent))
	}

	// (생략 없이 기존 View 로직 유지)
	if m.selectingStep == 10 {
		modalContent := "저장된 메모 파일 불러오기 (Load Memo)\n\n"
		if len(m.memoFiles) == 0 {
			modalContent += "저장된 메모 파일이 없습니다.\n"
		} else {
			for i, fName := range m.memoFiles {
				cursor := "  "
				if i == m.memoCursor {
					cursor = "> "
					fName = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Render(fName)
				}
				modalContent += fmt.Sprintf("%s%s\n", cursor, fName)
			}
		}
		modalContent += "\n[상/하] 이동  [Enter] 불러오기  [Esc] 취소"
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Render(modalContent))
	}

	if m.selectingStep == 1 {
		modalContent := "포트 선택 (Serial Port)\n\n"
		if len(m.availablePorts) == 0 {
			modalContent += "검색된 포트가 없습니다.\n"
		} else {
			for i, pName := range m.availablePorts {
				cursor := "  "
				if i == m.portCursor {
					cursor = "> "
					pName = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Render(pName)
				}
				modalContent += fmt.Sprintf("%s%s\n", cursor, pName)
			}
		}
		modalContent += "\n[상/하] 이동  [Enter] 다음  [Esc] 취소"
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Render(modalContent))
	}

	if m.selectingStep == 2 {
		modalContent := "통신 속도 선택 (Baud Rate)\n\n"
		for i, bRate := range m.baudRates {
			cursor := "  "
			rateStr := fmt.Sprintf("%d", bRate)
			if i == m.baudCursor {
				cursor = "> "
				rateStr = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Render(rateStr)
			}
			modalContent += fmt.Sprintf("%s%s\n", cursor, rateStr)
		}
		modalContent += "\n[상/하] 이동  [Enter] 다음(줄끝 설정)  [Esc] 취소"
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Render(modalContent))
	}

	if m.selectingStep == 9 {
		modalContent := "줄 끝 (Line Ending) 선택\n\n"
		for i, le := range m.lineEndings {
			cursor := "  "
			leStr := le
			if i == m.lineEndCursor {
				cursor = "> "
				leStr = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Render(leStr)
			}
			modalContent += fmt.Sprintf("%s%s\n", cursor, leStr)
		}
		modalContent += "\n[상/하] 이동  [Enter] 연결 완료  [Esc] 취소"
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Render(modalContent))
	}

	if m.selectingStep == 3 {
		s := "설정 메뉴 (Settings)\n\n"

		tsStatus := "[OFF]"
		if m.showTimestamp {
			tsStatus = "[ON]"
		}
		item1 := fmt.Sprintf("타임스탬프 사용 %s", tsStatus)
		if m.settingCursor == 0 {
			item1 = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Render(item1)
			s += fmt.Sprintf("> %s\n", item1)
		} else {
			s += fmt.Sprintf("  %s\n", item1)
		}

		asStatus := "[OFF]"
		if m.autoScroll {
			asStatus = "[ON]"
		}
		item2 := fmt.Sprintf("오토스크롤 사용 %s", asStatus)
		if m.settingCursor == 1 {
			item2 = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Render(item2)
			s += fmt.Sprintf("> %s\n", item2)
		} else {
			s += fmt.Sprintf("  %s\n", item2)
		}

		lgStatus := "[OFF]"
		if m.isLogging {
			lgStatus = "[ON]"
		}
		item3 := fmt.Sprintf("파일 로깅 사용 %s", lgStatus)
		if m.settingCursor == 2 {
			item3 = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Render(item3)
			s += fmt.Sprintf("> %s\n", item3)
		} else {
			s += fmt.Sprintf("  %s\n", item3)
		}

		hxStatus := "[OFF]"
		if m.isHexMode {
			hxStatus = "[ON]"
		}
		item4 := fmt.Sprintf("Hex 뷰어 모드 %s", hxStatus)
		if m.settingCursor == 3 {
			item4 = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Render(item4)
			s += fmt.Sprintf("> %s\n", item4)
		} else {
			s += fmt.Sprintf("  %s\n", item4)
		}

		item5 := fmt.Sprintf("줄 끝 처리: [%s]", m.lineEnding)
		if m.settingCursor == 4 {
			item5 = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Render(item5)
			s += fmt.Sprintf("> %s\n", item5)
		} else {
			s += fmt.Sprintf("  %s\n", item5)
		}

		themeName := "Cyberpunk"
		if m.theme == 1 {
			themeName = "Ocean Blue"
		} else if m.theme == 2 {
			themeName = "Amber Retro"
		}
		item6 := fmt.Sprintf("테마 변경: [%s]", themeName)
		if m.settingCursor == 5 {
			item6 = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Render(item6)
			s += fmt.Sprintf("> %s\n", item6)
		} else {
			s += fmt.Sprintf("  %s\n", item6)
		}

		item7 := fmt.Sprintf("폰트 설정: [%s, %dpt]", m.fontFamily, m.fontSize)
		if m.settingCursor == 6 {
			item7 = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Render(item7)
			s += fmt.Sprintf("> %s\n", item7)
		} else {
			s += fmt.Sprintf("  %s\n", item7)
		}

		item8 := fmt.Sprintf("로그 저장 폴더: %s", m.logDirPath)
		if m.settingCursor == 7 {
			item8 = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Render(item8)
			s += fmt.Sprintf("> %s\n", item8)
		} else {
			s += fmt.Sprintf("  %s\n", item8)
		}

		s += "\n[상/하] 이동  [Enter] 토글/설정  [Esc] 닫기"
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Render(s))
	}

	if m.selectingStep == 4 {
		s := "하이라이트 키워드 설정 (최대 10개)\n\n"
		if m.hlInputMode {
			if m.hlInputStep == 0 {
				s += fmt.Sprintf("새 키워드 입력:\n%s\n\n[Enter] 다음  [Esc] 취소", m.textInput.View())
			} else {
				s += fmt.Sprintf("키워드 [%s]의 색상 입력 (red, green, yellow, blue, cyan):\n%s\n\n[Enter] 추가 완료  [Esc] 취소", m.tempKeyword, m.textInput.View())
			}
		} else {
			if len(m.highlightRules) == 0 {
				s += "  등록된 키워드가 없습니다.\n\n"
			} else {
				for i, rule := range m.highlightRules {
					cursor := "  "
					lineStr := fmt.Sprintf("[%d] %s (%s)", i+1, rule.Keyword, rule.Color)
					if i == m.hlCursor {
						cursor = "> "
						lineStr = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Render(lineStr)
					}
					s += fmt.Sprintf("%s%s (선택 시 삭제)\n", cursor, lineStr)
				}
			}

			if len(m.highlightRules) < 10 {
				addCursor := "  "
				addStr := "+ 새 키워드 추가하기"
				if m.hlCursor == len(m.highlightRules) {
					addCursor = "> "
					addStr = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Render(addStr)
				}
				s += fmt.Sprintf("\n%s%s\n", addCursor, addStr)
			} else {
				s += "\n(최대 10개까지 등록되었습니다)\n"
			}
			s += "\n[상/하] 이동  [Enter] 추가/삭제  [Esc] 닫기"
		}
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Render(s))
	}

	if m.selectingStep == 5 {
		s := "로그 저장 폴더 경로 입력\n\n"
		s += fmt.Sprintf("저장할 폴더 경로를 입력하세요:\n%s\n\n[Enter] 저장  [Esc] 취소", m.textInput.View())
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Render(s))
	}

	if m.selectingStep == 6 {
		s := "로그 키워드 검색\n\n"
		s += fmt.Sprintf("검색할 단어 입력 (비우고 엔터 시 필터 해제):\n%s\n\n[Enter] 검색/해제  [Esc] 취소", m.textInput.View())
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Render(s))
	}

	if m.selectingStep == 7 {
		s := "전체 로그 내보내기 (Export)\n\n"
		s += fmt.Sprintf("저장할 파일명을 입력하세요:\n%s\n\n[Enter] 내보내기  [Esc] 취소", m.textInput.View())
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Render(s))
	}

	if m.selectingStep == 8 {
		s := "폰트 설정 (Font Family & Size)\n\n"
		s += fmt.Sprintf("폰트 이름과 크기를 입력하세요 (예: JetBrains Mono, 14):\n%s\n\n[Enter] 설정 완료  [Esc] 취소", m.textInput.View())
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ts.Modal.Render(s))
	}

	title := ts.Title.Render("muart Terminal")

	statusStr := "CONNECTED"
	if m.port == nil {
		statusStr = "DISCONNECTED"
	}

	statusStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Render(statusStr)
	if m.port == nil {
		statusStyled = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(statusStr)
	}

	tsCheck := "[x]"
	if m.showTimestamp {
		tsCheck = "[✓]"
	}

	asCheck := "[x]"
	if m.autoScroll {
		asCheck = "[✓]"
	}

	lgCheck := "[x]"
	if m.isLogging {
		lgCheck = "[✓]"
	}

	hxCheck := "[x]"
	if m.isHexMode {
		hxCheck = "[✓]"
	}

	searchInfo := ""
	if m.searchQuery != "" {
		searchInfo = fmt.Sprintf(" | 검색: \"%s\"", m.searchQuery)
	}

	modeStr := lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).Render("INPUT")
	if !m.inInputMode {
		modeStr = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true).Render("MODE (Command)")
	}

	statInfo := fmt.Sprintf(" | RX: %dB (%d pkts) | TX: %dB (%d pkts)", m.rxBytes, m.rxPkts, m.txBytes, m.txPkts)
	fontInfo := fmt.Sprintf(" | 폰트: %s (%dpt)", m.fontFamily, m.fontSize)

	footerText := fmt.Sprintf(" 상태: %s | 포트: %s | Baud: %d | 줄끝: %s | TS %s | AS %s | LOG %s | HEX %s%s%s%s | [%s] ", statusStyled, m.portName, m.baudRate, m.lineEnding, tsCheck, asCheck, lgCheck, hxCheck, searchInfo, statInfo, fontInfo, modeStr)

	appWidth := m.width
	if appWidth <= 0 {
		appWidth = 80
	}

	boxWidth := appWidth - 4
	if boxWidth < 30 {
		boxWidth = 30
	}

	footerBox := ts.Footer.Width(boxWidth).Render(footerText)
	help := ts.Help.Render("[v] 화면분할 [Tab] 포커스전환 [Ctrl+s] 저장 [Ctrl+o] 불러오기 [Esc] 모드전환 [p] 포트 [s] 설정 [c] 클리어 [?] 도움말")

	var mainView string
	if m.showSplit {
		leftWidth := (boxWidth / 2) - 1
		rightWidth := boxWidth - leftWidth - 2

		m.viewport.Width = leftWidth - 2
		m.memoArea.SetWidth(rightWidth - 4)

		leftBoxStyle := ts.Box
		rightBoxStyle := ts.Box

		if m.activePane == 0 {
			leftBoxStyle = leftBoxStyle.BorderForeground(lipgloss.Color("212")).Background(lipgloss.Color("235"))
		} else {
			rightBoxStyle = rightBoxStyle.BorderForeground(lipgloss.Color("212")).Background(lipgloss.Color("235"))
		}

		leftBox := leftBoxStyle.Width(leftWidth).Render(m.viewport.View())
		rightBox := rightBoxStyle.Width(rightWidth).Render(m.memoArea.View())

		mainView = lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)
	} else {
		m.viewport.Width = boxWidth - 2
		mainView = ts.Box.Width(boxWidth).Render(m.viewport.View())
	}

	contentArea := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		mainView,
		footerBox,
		help,
	)

	h := m.height
	if h <= 0 {
		h = 24
	}

	return ts.AppBorder.Width(appWidth - 2).Height(h - 2).Render(contentArea)
}

func main() {
	cfg := loadConfig()

	portName := cfg.PortName
	baudRate := cfg.BaudRate
	lineEnding := cfg.LineEnding

	standardBaudRates := []int{9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600}
	standardLineEndings := []string{"없음", "LF", "CR", "CRLF"}

	mode := &serial.Mode{
		BaudRate: baudRate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(portName, mode)

	asciiArtLogo := `
  __  __ _   _    _     ____ _____ 
 |  \/  | | | |  / \   |  _ \_   _|
 | |\/| | | | | / _ \  | |_) || |  
 | |  | | |_| |/ ___ \ |  _ < | |  
 |_|  |_|\___//_/   \_\|_| \_\|_|  
`

	initialRules := cfg.HighlightRules
	initialLogText := fmt.Sprintf("%s\nmuart 시리얼 포트 연결 대기 중...\n'p'를 눌러 포트와 보드레이트를 설정하세요.", asciiArtLogo)
	if err != nil {
		initialLogText = fmt.Sprintf("%s\n[에러] 기본 포트(%s)를 열 수 없습니다: %v\n'p'를 눌러 사용 가능한 포트를 선택하세요.", asciiArtLogo, portName, err)
		port = nil
	}

	ti := textinput.New()
	ti.Placeholder = "명령어를 입력하세요 (Hex 전송 예: hex: DE AD)"
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 60
	ti.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).Render("> ")

	ma := textarea.New()
	ma.Placeholder = "메모를 작성하거나 불러오세요..."
	ma.Blur()
	ma.SetValue("메모장 창\n\n[Tab] 키로 좌우 포커스 전환 가능\n[Ctrl+s] 파일저장 / [Ctrl+o] 파일불러오기")

	vp := viewport.New(80, 18)
	styledInitialLog := highlightLog(initialLogText, initialRules)
	vp.SetContent(styledInitialLog + "\n" + ti.View())

	initialModel := model{
		port:            port,
		portName:        portName,
		baudRate:        baudRate,
		lineEnding:      lineEnding,
		viewport:        vp,
		memoArea:        ma,
		textInput:       ti,
		activePane:      0,
		showSplit:       false,
		logBuffer:       styledInitialLog,
		fullLogBuffer:   styledInitialLog,
		displayLogLines: strings.Split(styledInitialLog, "\n"),
		displayLogBytes: len(styledInitialLog),
		searchQuery:     "",
		cmdHistory:      []string{},
		historyIndex:    -1,
		rxBytes:         0,
		rxPkts:          0,
		txBytes:         0,
		txPkts:          0,
		baudRates:       standardBaudRates,
		baudCursor:      4,
		lineEndings:     standardLineEndings,
		lineEndCursor:   3,
		showTimestamp:   cfg.ShowTimestamp,
		autoScroll:      cfg.AutoScroll,
		isLogging:       false,
		isHexMode:       cfg.IsHexMode,
		theme:           cfg.Theme,
		fontFamily:      cfg.FontFamily,
		fontSize:        cfg.FontSize,
		logDirPath:      cfg.LogDirPath,
		logFile:         nil,
		inInputMode:     true,
		highlightRules:  cfg.HighlightRules,
	}

	// tea.WithMouseCellMotion()을 추가하여 마우스 스크롤 이벤트 활성화
	p := tea.NewProgram(initialModel, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		log.Fatalf("프로그램 실행 오류: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
}
