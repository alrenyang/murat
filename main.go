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

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"go.bug.st/serial"
)

const configFile = "muart_config.json"

type Config struct {
	PortName       string          `json:"port_name"`
	BaudRate       int             `json:"baud_rate"`
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

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultCfg
	}
	return cfg
}

func (m *model) saveConfig() {
	cfg := Config{
		PortName:       m.portName,
		BaudRate:       m.baudRate,
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
	if err == nil {
		_ = os.WriteFile(configFile, data, 0644)
	}
}

type serialMsg string

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
			Box:       lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("51")).Padding(0, 1),
			Footer:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("75")).Padding(0, 1),
			Title:     lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true).Italic(true),
			Help:      lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
			Modal:     lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(lipgloss.Color("39")).Padding(1, 3).Width(55),
		}
	case 2: // Amber Retro
		return ThemeStyles{
			AppBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("208")),
			Box:       lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("214")).Padding(0, 1),
			Footer:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("202")).Padding(0, 1),
			Title:     lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Italic(true),
			Help:      lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
			Modal:     lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(lipgloss.Color("208")).Padding(1, 3).Width(55),
		}
	default: // 0: Cyberpunk (Default)
		return ThemeStyles{
			AppBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("99")),
			Box:       lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("42")).Padding(0, 1),
			Footer:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("39")).Padding(0, 1),
			Title:     lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Italic(true),
			Help:      lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
			Modal:     lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(lipgloss.Color("214")).Padding(1, 3).Width(55),
		}
	}
}

type model struct {
	port          serial.Port
	portName      string
	baudRate      int
	viewport      viewport.Model
	textInput     textinput.Model
	logBuffer     string
	fullLogBuffer string
	searchQuery   string
	width         int
	height        int

	cmdHistory   []string
	historyIndex int

	rxBytes int64
	rxPkts  int64
	txBytes int64
	txPkts  int64

	selectingStep  int
	availablePorts []string
	portCursor     int

	baudRates     []int
	baudCursor    int
	tempPortName  string
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

	if m.isHexMode && !strings.HasPrefix(raw, "[설정]") && !strings.HasPrefix(raw, "[에러]") && !strings.HasPrefix(raw, "[로그") {
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

func (m *model) updateViewportContent() {
	content := m.fullLogBuffer
	if m.searchQuery != "" {
		lines := strings.Split(m.fullLogBuffer, "\n")
		var filtered []string
		for _, line := range lines {
			if strings.Contains(strings.ToUpper(line), strings.ToUpper(m.searchQuery)) {
				filtered = append(filtered, line)
			}
		}
		if len(filtered) == 0 {
			content = fmt.Sprintf("[검색 결과 없음: \"%s\"]", m.searchQuery)
		} else {
			content = strings.Join(filtered, "\n")
		}
	}

	fullDisplay := content + "\n" + m.textInput.View()
	m.viewport.SetContent(fullDisplay)
	if m.autoScroll {
		m.viewport.GotoBottom()
	}
}

func (m *model) writeLog(rawText string) {
	m.rxBytes += int64(len(rawText))
	m.rxPkts++

	displayLine, fileLine := m.formatLogData(rawText)
	finalDisplay := highlightLog(displayLine, m.highlightRules)

	if m.fullLogBuffer == "" {
		m.fullLogBuffer = finalDisplay
	} else {
		m.fullLogBuffer += "\n" + finalDisplay
	}

	if m.isLogging && m.logFile != nil {
		m.logFile.WriteString(fileLine + "\n")
	}

	m.updateViewportContent()
}

func (m *model) writeTxLog(rawText string) {
	displayLine, fileLine := m.formatLogData(rawText)
	finalDisplay := lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render(displayLine)

	if m.fullLogBuffer == "" {
		m.fullLogBuffer = finalDisplay
	} else {
		m.fullLogBuffer += "\n" + finalDisplay
	}

	if m.isLogging && m.logFile != nil {
		m.logFile.WriteString(fileLine + "\n")
	}

	m.updateViewportContent()
}

func readSerial(p serial.Port) tea.Cmd {
	return func() tea.Msg {
		buf := make([]byte, 128)
		n, err := p.Read(buf)
		if err != nil {
			return serialMsg(fmt.Sprintf("[에러] 읽기 실패: %v", err))
		}
		if n > 0 {
			return serialMsg(string(buf[:n]))
		}
		return nil
	}
}

func (m model) Init() tea.Cmd {
	if m.port != nil {
		return tea.Batch(readSerial(m.port), textinput.Blink)
	}
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.selectingStep == 1 {
			switch msg.String() {
			case "esc", "q":
				m.selectingStep = 0
				m.textInput.Reset() // 입력창 초기화
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
				selectedBaud := m.baudRates[m.baudCursor]
				if m.port != nil {
					m.port.Close()
				}
				m.portName = m.tempPortName
				m.baudRate = selectedBaud

				mode := &serial.Mode{
					BaudRate: m.baudRate,
					Parity:   serial.NoParity,
					DataBits: 8,
					StopBits: serial.OneStopBit,
				}
				p, err := serial.Open(m.portName, mode)
				if err != nil {
					m.port = nil
					m.writeLog(fmt.Sprintf("[에러] 포트 열기 실패 (%s): %v", m.portName, err))
				} else {
					m.port = p
					m.writeLog(fmt.Sprintf("[설정 변경] 포트: %s, 보드레이트: %d 연결 성공", m.portName, m.baudRate))
				}

				m.saveConfig()

				m.selectingStep = 0
				m.textInput.Reset()
				if m.inInputMode {
					m.textInput.Focus()
				}
				if m.port != nil {
					return m, readSerial(m.port)
				}
			}
			return m, nil
		}

		if m.selectingStep == 3 {
			switch msg.String() {
			case "esc", "q":
				m.selectingStep = 0
				m.textInput.Reset() // 설정창 닫을 때 입력창 값 클리어
				if m.inInputMode {
					m.textInput.Focus()
				}
			case "up", "k":
				if m.settingCursor > 0 {
					m.settingCursor--
				}
			case "down", "j":
				if m.settingCursor < 6 {
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
							m.logFile.Close()
							m.logFile = nil
						}
						m.isLogging = false
						m.writeLog("[설정] 파일 로깅 기능: OFF [x]")
					} else {
						if err := os.MkdirAll(m.logDirPath, 0755); err != nil {
							m.writeLog(fmt.Sprintf("[에러] 로그 폴더 생성 실패 (%s): %v", m.logDirPath, err))
							return m, nil
						}

						filename := filepath.Join(m.logDirPath, fmt.Sprintf("muart_log_%s.txt", time.Now().Format("20060102_150405")))
						f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
						if err != nil {
							m.writeLog(fmt.Sprintf("[에러] 로그 파일 생성 실패: %v", err))
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
					m.theme = (m.theme + 1) % 3
					themeName := "Cyberpunk"
					if m.theme == 1 {
						themeName = "Ocean Blue"
					} else if m.theme == 2 {
						themeName = "Amber Retro"
					}
					m.writeLog(fmt.Sprintf("[설정] 테마 변경: %s", themeName))
				} else if m.settingCursor == 5 {
					m.selectingStep = 8
					m.textInput.Reset()
					m.textInput.Placeholder = "예: JetBrains Mono, 14"
					m.textInput.SetValue(fmt.Sprintf("%s, %d", m.fontFamily, m.fontSize))
					m.textInput.Focus()
					return m, nil
				} else if m.settingCursor == 6 {
					m.selectingStep = 5
					m.textInput.Reset()
					m.textInput.Placeholder = "예: ./logs 또는 /Users/name/Desktop"
					m.textInput.SetValue(m.logDirPath)
					m.textInput.Focus()
					return m, nil
				}
				m.saveConfig()
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
							m.saveConfig()
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
					m.saveConfig()
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
					m.saveConfig()
					if m.isLogging {
						if m.logFile != nil {
							m.logFile.Close()
							m.logFile = nil
						}
						if err := os.MkdirAll(m.logDirPath, 0755); err == nil {
							filename := filepath.Join(m.logDirPath, fmt.Sprintf("muart_log_%s.txt", time.Now().Format("20060102_150405")))
							if f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
								m.logFile = f
								m.writeLog(fmt.Sprintf("[설정] 로그 저장 폴더 변경 및 파일 재연결 완료: %s", filename))
							} else {
								m.isLogging = false
								m.writeLog(fmt.Sprintf("[에러] 새 경로 파일 생성 실패: %v", err))
							}
						} else {
							m.isLogging = false
							m.writeLog(fmt.Sprintf("[에러] 폴더 생성 실패: %v", err))
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
						m.writeLog(fmt.Sprintf("[에러] 로그 내보내기 실패: %v", err))
					} else {
						m.writeLog(fmt.Sprintf("[설정] 로그가 성공적으로 내보내기 되었습니다: %s", fullExportPath))
					}
				} else {
					m.writeLog(fmt.Sprintf("[에러] 내보내기 폴더 생성 실패: %v", err))
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
					m.saveConfig()
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
		case "esc":
			m.inInputMode = !m.inInputMode
			if m.inInputMode {
				m.textInput.Focus()
				m.textInput.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).Render("> ")
			} else {
				m.textInput.Blur()
				m.textInput.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true).Render("(cmd)> ")
			}
			m.updateViewportContent()
			return m, nil

		case "ctrl+c":
			if m.port != nil {
				m.port.Close()
			}
			if m.logFile != nil {
				m.logFile.Close()
			}
			return m, tea.Quit
		}

		if m.inInputMode {
			switch msg.String() {
			case "enter":
				typedText := m.textInput.Value()
				if typedText != "" {
					m.txBytes += int64(len(typedText) + 2)
					m.txPkts++

					m.cmdHistory = append(m.cmdHistory, typedText)
					m.historyIndex = -1

					if m.port != nil {
						_, err := m.port.Write([]byte(typedText + "\r\n"))
						if err != nil {
							m.writeLog(fmt.Sprintf("[송신 FAIL] %v", err))
						} else {
							m.writeTxLog(typedText)
						}
					} else {
						m.writeLog("[에러] 연결된 시리얼 포트가 없습니다. 'Esc'를 눌러 모드 전환 후 'p'를 눌러 포트를 선택하세요.")
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
					m.port.Close()
					m.port = nil
				}
				if m.portName == "" {
					m.writeLog("[에러] 설정된 포트 이름이 없습니다. 'p'를 눌러 포트를 선택하세요.")
					return m, nil
				}
				mode := &serial.Mode{
					BaudRate: m.baudRate,
					Parity:   serial.NoParity,
					DataBits: 8,
					StopBits: serial.OneStopBit,
				}
				p, err := serial.Open(m.portName, mode)
				if err != nil {
					m.port = nil
					m.writeLog(fmt.Sprintf("[에러] 재연결 실패 (%s): %v", m.portName, err))
					return m, nil
				} else {
					m.port = p
					m.writeLog(fmt.Sprintf("[설정] 시리얼 포트 재연결 성공: %s (%d)", m.portName, m.baudRate))
					return m, readSerial(m.port)
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
				m.fullLogBuffer = highlightLog(displayLine, m.highlightRules)
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
			case "down", "j":
				m.viewport.LineDown(1)
			}
		}

	case serialMsg:
		m.writeLog(string(msg))
		return m, readSerial(m.port)

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
		m.viewport.Width = boxWidth - 2
		m.viewport.Height = m.height - 10
		m.textInput.Width = boxWidth - 6
	}

	if m.selectingStep == 0 && m.inInputMode {
		m.textInput, cmd = m.textInput.Update(msg)
		m.updateViewportContent()
		m.viewport, _ = m.viewport.Update(msg)
		return m, cmd
	}

	m.viewport, _ = m.viewport.Update(msg)
	return m, cmd
}

func (m model) View() string {
	ts := getThemeStyles(m.theme)

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

		themeName := "Cyberpunk"
		if m.theme == 1 {
			themeName = "Ocean Blue"
		} else if m.theme == 2 {
			themeName = "Amber Retro"
		}
		item5 := fmt.Sprintf("테마 변경: [%s]", themeName)
		if m.settingCursor == 4 {
			item5 = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Render(item5)
			s += fmt.Sprintf("> %s\n", item5)
		} else {
			s += fmt.Sprintf("  %s\n", item5)
		}

		item6 := fmt.Sprintf("폰트 설정: [%s, %dpt]", m.fontFamily, m.fontSize)
		if m.settingCursor == 5 {
			item6 = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Render(item6)
			s += fmt.Sprintf("> %s\n", item6)
		} else {
			s += fmt.Sprintf("  %s\n", item6)
		}

		item7 := fmt.Sprintf("로그 저장 폴더: %s", m.logDirPath)
		if m.settingCursor == 6 {
			item7 = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Render(item7)
			s += fmt.Sprintf("> %s\n", item7)
		} else {
			s += fmt.Sprintf("  %s\n", item7)
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
	footerText := fmt.Sprintf(" 상태: %s | 포트: %s | Baud: %d | TS %s | AS %s | LOG %s | HEX %s%s%s%s | [%s] ", statusStyled, m.portName, m.baudRate, tsCheck, asCheck, lgCheck, hxCheck, searchInfo, statInfo, fontInfo, modeStr)

	appWidth := m.width
	if appWidth <= 0 {
		appWidth = 80
	}

	boxWidth := appWidth - 4
	if boxWidth < 30 {
		boxWidth = 30
	}

	footerBox := ts.Footer.Width(boxWidth).Render(footerText)
	help := ts.Help.Render("[Esc] 모드전환    [r] 재연결    [e] 내보내기    [/] 검색    [p] 포트    [s] 설정    [h] 하이라이트    [c] 클리어")

	m.viewport.Width = boxWidth - 2
	m.textInput.Width = boxWidth - 6
	mainView := ts.Box.Width(boxWidth).Render(m.viewport.View())

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

	standardBaudRates := []int{9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600}

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
	ti.Placeholder = "명령어를 입력하세요 (예: AT+VERSION)"
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 60
	ti.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).Render("> ")

	vp := viewport.New(80, 18)
	styledInitialLog := highlightLog(initialLogText, initialRules)
	vp.SetContent(styledInitialLog + "\n" + ti.View())

	initialModel := model{
		port:           port,
		portName:       portName,
		baudRate:       baudRate,
		viewport:       vp,
		textInput:      ti,
		logBuffer:      styledInitialLog,
		fullLogBuffer:  styledInitialLog,
		searchQuery:    "",
		cmdHistory:     []string{},
		historyIndex:   -1,
		rxBytes:        0,
		rxPkts:         0,
		txBytes:        0,
		txPkts:         0,
		baudRates:      standardBaudRates,
		baudCursor:     4,
		showTimestamp:  cfg.ShowTimestamp,
		autoScroll:     cfg.AutoScroll,
		isLogging:      false,
		isHexMode:      cfg.IsHexMode,
		theme:          cfg.Theme,
		fontFamily:     cfg.FontFamily,
		fontSize:       cfg.FontSize,
		logDirPath:     cfg.LogDirPath,
		logFile:        nil,
		inInputMode:    true,
		highlightRules: cfg.HighlightRules,
	}

	p := tea.NewProgram(initialModel, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatalf("프로그램 실행 오류: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
}
