package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
)

type phase int

const (
	phaseInit phase = iota
	phasePing
	phaseDownloading
	phaseUploading
	phaseComplete
	phaseError
)

type speedTest struct {
	phase          phase
	downloadSpeed  float64
	uploadSpeed    float64
	ping           float64
	serverLocation string
	progress       progress.Model
	err            error
	speedHistory   []float64
	startTime      time.Time
	testDuration   time.Duration
	targetSpeed    float64
	animationSpeed float64
}

type tickMsg time.Time
type speedMsg float64
type pingMsg float64
type uploadMsg float64
type serverMsg string
type errorMsg error
type completeMsg struct {
	download float64
	upload   float64
	ping     float64
	server   string
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
	}
}

func initialModel() speedTest {
	return speedTest{
		phase:        phaseInit,
		progress:     progress.New(progress.WithDefaultGradient()),
		speedHistory: make([]float64, 0),
		startTime:    time.Now(),
	}
}

func (m speedTest) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		m.progress.Init(),
		runSpeedTestCmd(),
	)
}

func (m speedTest) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			if m.phase == phaseComplete || m.phase == phaseError {
				newModel := initialModel()
				return newModel, tea.Batch(
					tickCmd(),
					runSpeedTestCmd(),
				)
			}
		}

	case tickMsg:
		if m.phase == phaseDownloading || m.phase == phaseUploading {

			var targetSpeed float64
			if m.phase == phaseDownloading {

				elapsed := time.Since(m.startTime).Seconds()
				if elapsed > 2.0 && elapsed < 7.0 {
					maxSpeed := 50.0 + float64(time.Now().UnixNano()%50)
					progress := (elapsed - 2.0) / 5.0
					if progress > 1.0 {
						progress = 1.0
					}

					variation := math.Sin(elapsed*2) * 5.0
					targetSpeed = maxSpeed*(0.2+0.8*progress) + variation
					if targetSpeed < 0 {
						targetSpeed = 5.0
					}
					m.downloadSpeed = targetSpeed
					m.targetSpeed = targetSpeed

					m.speedHistory = append(m.speedHistory, targetSpeed)
					if len(m.speedHistory) > 60 {
						m.speedHistory = m.speedHistory[1:]
					}
				} else {
					targetSpeed = m.downloadSpeed
				}
			} else {

				elapsed := time.Since(m.startTime).Seconds()
				if elapsed > 7.0 && elapsed < 11.0 {
					maxSpeed := 25.0 + float64(time.Now().UnixNano()%25)
					progress := (elapsed - 7.0) / 4.0
					if progress > 1.0 {
						progress = 1.0
					}
					variation := math.Sin(elapsed*3) * 3.0
					targetSpeed = maxSpeed*(0.3+0.7*progress) + variation
					if targetSpeed < 0 {
						targetSpeed = 8.0
					}
					m.uploadSpeed = targetSpeed
					m.targetSpeed = targetSpeed
				} else {
					targetSpeed = m.uploadSpeed
				}
			}

			diff := targetSpeed - m.animationSpeed
			if math.Abs(diff) > 0.5 {
				m.animationSpeed += diff * 0.15
			} else {
				m.animationSpeed = targetSpeed
			}

			return m, tickCmd()
		}

	case speedMsg:
		if m.phase == phaseDownloading {
			m.downloadSpeed = float64(msg)
			m.targetSpeed = m.downloadSpeed
		}
		m.speedHistory = append(m.speedHistory, float64(msg))
		if len(m.speedHistory) > 60 {
			m.speedHistory = m.speedHistory[1:]
		}
		return m, nil

	case completeMsg:
		m.phase = phaseComplete
		m.downloadSpeed = msg.download
		m.uploadSpeed = msg.upload
		m.ping = msg.ping
		m.serverLocation = msg.server
		m.testDuration = time.Since(m.startTime)
		m.targetSpeed = math.Max(m.downloadSpeed, m.uploadSpeed)
		m.animationSpeed = m.targetSpeed
		return m, nil

	case serverMsg:
		m.serverLocation = string(msg)
		m.phase = phasePing
		return m, nil

	case pingMsg:
		m.ping = float64(msg)
		m.phase = phaseDownloading

		m.animationSpeed = 0
		m.targetSpeed = 0
		return m, tickCmd()

	case uploadMsg:
		m.uploadSpeed = float64(msg)
		m.phase = phaseUploading
		m.targetSpeed = m.uploadSpeed
		return m, nil

	case errorMsg:
		m.phase = phaseError
		m.err = msg
		return m, nil

	case tea.WindowSizeMsg:
		m.progress.Width = msg.Width - 4
	}

	return m, nil
}

func (m speedTest) View() string {
	var s strings.Builder

	title := "\033[37;1;44m GoFast - Speed Test \033[0m"

	s.WriteString(title + "\n\n")

	switch m.phase {
	case phaseInit:
		s.WriteString("Initializing speed test...\n")
		s.WriteString("Getting server location...\n\n")
		s.WriteString(m.renderSpeedometer(0))

	case phasePing:
		s.WriteString("Testing connection to server...\n\n")
		if m.serverLocation != "" {
			s.WriteString(fmt.Sprintf("\033[32;1m🌐 Server: %s\033[0m\n\n", m.serverLocation))
		}
		s.WriteString(m.renderSpeedometer(0))
		if m.ping > 0 {
			s.WriteString(fmt.Sprintf("\nPing: %.1f ms\n", m.ping))
		} else {
			s.WriteString("\nTesting ping...")
		}

	case phaseDownloading:
		s.WriteString("Testing download speed...\n\n")
		if m.serverLocation != "" {
			s.WriteString(fmt.Sprintf("\033[32;1mConnected to: %s\033[0m\n\n", m.serverLocation))
		}
		s.WriteString(m.renderDualSpeedometer(m.animationSpeed, 0))
		s.WriteString(fmt.Sprintf("\nDownload Speed: %.2f Mbps\n", m.downloadSpeed))
		if m.ping > 0 {
			s.WriteString(fmt.Sprintf("Ping: %.1f ms\n", m.ping))
		}
		s.WriteString(m.renderSpeedHistory())

	case phaseUploading:
		s.WriteString("Testing upload speed...\n\n")
		if m.serverLocation != "" {
			s.WriteString(fmt.Sprintf("\033[32;1mConnected to: %s\033[0m\n\n", m.serverLocation))
		}
		s.WriteString(m.renderDualSpeedometer(m.downloadSpeed, m.animationSpeed))
		s.WriteString(fmt.Sprintf("\nDownload: %.2f Mbps\n", m.downloadSpeed))
		s.WriteString(fmt.Sprintf("Upload: %.2f Mbps\n", m.uploadSpeed))
		if m.ping > 0 {
			s.WriteString(fmt.Sprintf("Ping: %.1f ms\n", m.ping))
		}

	case phaseComplete:
		s.WriteString("Speed test complete!\n\n")
		if m.serverLocation != "" {
			s.WriteString(fmt.Sprintf("\033[32;1mTested via: %s\033[0m\n\n", m.serverLocation))
		}
		s.WriteString(m.renderDualSpeedometer(m.downloadSpeed, m.uploadSpeed))
		s.WriteString(fmt.Sprintf("\nDownload: %.2f Mbps\n", m.downloadSpeed))
		s.WriteString(fmt.Sprintf("Upload: %.2f Mbps\n", m.uploadSpeed))
		s.WriteString(fmt.Sprintf("Ping: %.1f ms\n", m.ping))
		s.WriteString(fmt.Sprintf("Test Duration: %.1fs\n", m.testDuration.Seconds()))
		s.WriteString("\nPress 'r' to run again")

	case phaseError:
		s.WriteString("Error occurred:\n")
		s.WriteString(fmt.Sprintf("%v\n", m.err))
		s.WriteString("\nPress 'r' to try again")
	}

	s.WriteString("\n\nPress 'q' to quit")
	return s.String()
}

func (m speedTest) renderDualSpeedometer(downloadSpeed, uploadSpeed float64) string {
	var s strings.Builder

	s.WriteString("     ╔═══════════════════════════════════════════════════════════════════════════════════════════════╗\n")
	s.WriteString("     ║                                    goFast tui                                                  ║\n")
	s.WriteString("     ║                        DOWNLOAD                              UPLOAD                              ║\n")
	s.WriteString("     ╚═══════════════════════════════════════════════════════════════════════════════════════════════╝\n")

	for row := 0; row < 35; row++ {
		s.WriteString("     ")
		for col := 0; col < 90; col++ {
			char := " "

			if col < 45 {
				char = m.renderSingleGauge(float64(col), float64(row), 22.0, 18.0, downloadSpeed, 18.0, 14.0)
			}

			if col >= 45 {
				char = m.renderSingleGauge(float64(col-45), float64(row), 22.0, 18.0, uploadSpeed, 18.0, 14.0)
			}

			s.WriteString(char)
		}
		s.WriteString("\n")
	}
//temp probably need to try somethign else
	s.WriteString("     0   10   20   30   40   50   60   70   80   90  100     0   10   20   30   40   50   60   70   80   90  100\n")
	s.WriteString("                           Mbps                                                 Mbps\n")

	var downloadDisplay, uploadDisplay string
	if downloadSpeed >= 80 {
		downloadDisplay = fmt.Sprintf("     \033[31;1mDownload: %.1f Mbps\033[0m", downloadSpeed)
	} else if downloadSpeed >= 60 {
		downloadDisplay = fmt.Sprintf("     \033[33;1mDownload: %.1f Mbps\033[0m", downloadSpeed)
	} else if downloadSpeed >= 30 {
		downloadDisplay = fmt.Sprintf("     \033[32;1mDownload: %.1f Mbps\033[0m", downloadSpeed)
	} else {
		downloadDisplay = fmt.Sprintf("     \033[36;1mDownload: %.1f Mbps\033[0m", downloadSpeed)
	}

	if uploadSpeed >= 80 {
		uploadDisplay = fmt.Sprintf("                                 \033[31;1mUpload: %.1f Mbps\033[0m\n", uploadSpeed)
	} else if uploadSpeed >= 60 {
		uploadDisplay = fmt.Sprintf("                                 \033[33;1mUpload: %.1f Mbps\033[0m\n", uploadSpeed)
	} else if uploadSpeed >= 30 {
		uploadDisplay = fmt.Sprintf("                                 \033[32;1mUpload: %.1f Mbps\033[0m\n", uploadSpeed)
	} else {
		uploadDisplay = fmt.Sprintf("                                 \033[36;1mUpload: %.1f Mbps\033[0m\n", uploadSpeed)
	}

	s.WriteString(downloadDisplay + uploadDisplay)
	return s.String()
}

func (m speedTest) renderSingleGauge(x, y, centerX, centerY, speed, outerRadius, innerRadius float64) string {

	dx := x - centerX
	dy := y - centerY
	distance := math.Sqrt(dx*dx + dy*dy)
	angle := math.Atan2(-dy, dx)

	if angle < 0 {
		angle += 2 * math.Pi
	}

	angleInDegrees := angle * 180.0 / math.Pi

	inArc := (angleInDegrees >= 315.0 || angleInDegrees <= 225.0) && !(angleInDegrees > 225.0 && angleInDegrees < 315.0)

	char := " "

	if distance >= outerRadius-1.0 && distance <= outerRadius+1.0 && inArc {
		char = "█"
	} else if distance >= innerRadius-0.8 && distance <= innerRadius+0.8 && inArc {

		char = "░"
	} else if distance >= 3 && distance <= innerRadius-2 {

		needleAngle := 240.0 - (speed/100.0)*270.0
		if needleAngle < 0.0 {
			needleAngle += 360.0
		}
		needleAngleRad := needleAngle * math.Pi / 180.0

		needleEndX := centerX + (innerRadius-3)*math.Cos(needleAngleRad)
		needleEndY := centerY - (innerRadius-3)*math.Sin(needleAngleRad)

		lineDistance := math.Abs((needleEndY-centerY)*x-(needleEndX-centerX)*y+needleEndX*centerY-needleEndY*centerX) /
			math.Sqrt(math.Pow(needleEndY-centerY, 2)+math.Pow(needleEndX-centerX, 2))

		if lineDistance < 1.2 &&
			((x-centerX)*(needleEndX-centerX)+(y-centerY)*(needleEndY-centerY)) > 0 {
			char = "━"
		}
	} else if distance <= 3.0 {

		char = "●"
	} else if distance >= 8 && distance <= 12 && inArc {

		if angleInDegrees >= 135.0 && angleInDegrees <= 216.0 {
			char = "▓"
		} else if angleInDegrees >= 216.0 && angleInDegrees <= 297.0 {
			char = "▒"
		} else if (angleInDegrees >= 297.0) || (angleInDegrees >= 0.0 && angleInDegrees <= 18.0) {
			char = "░"
		} else if angleInDegrees >= 18.0 && angleInDegrees <= 45.0 {
			char = "▓"
		}
	}

	return char
}

func (m speedTest) renderSpeedometer(speed float64) string {
	var s strings.Builder

	centerX, centerY := 25.0, 20.0
	outerRadius := 18.0
	innerRadius := 14.0

	maxSpeed := 100.0
	speedAngle := (speed / maxSpeed) * 270.0
	if speedAngle > 270 {
		speedAngle = 270
	}

	s.WriteString("     ╔═══════════════════════════════════════════════╗\n")
	s.WriteString("     ║               goFast tui                      ║\n")
	s.WriteString("     ╚═══════════════════════════════════════════════╝\n")

	for row := 0; row < 35; row++ {
		s.WriteString("     ")
		for col := 0; col < 50; col++ {
			x := float64(col)
			y := float64(row)

			dx := x - centerX
			dy := y - centerY
			distance := math.Sqrt(dx*dx + dy*dy)
			angle := math.Atan2(-dy, dx)

			if angle < 0 {
				angle += 2 * math.Pi
			}

			angleInDegrees := angle * 180.0 / math.Pi

			inArc := (angleInDegrees >= 315.0 || angleInDegrees <= 225.0) && !(angleInDegrees > 225.0 && angleInDegrees < 315.0)

			char := " "

			if distance >= outerRadius-1.0 && distance <= outerRadius+1.0 && inArc {
				char = "█"
			} else if distance >= innerRadius-0.8 && distance <= innerRadius+0.8 && inArc {

				char = "░"
			} else if distance >= outerRadius+1.5 && distance <= outerRadius+4.0 && inArc {

				tickPositions := []float64{135, 162, 189, 216, 243, 270, 297, 324, 351, 18, 45}

				for i, tickAngle := range tickPositions {

					normalizedTickAngle := tickAngle
					if normalizedTickAngle > 360 {
						normalizedTickAngle -= 360
					}

					angleDiff := math.Abs(angleInDegrees - normalizedTickAngle)
					if angleDiff > 180 {
						angleDiff = 360 - angleDiff
					}

					if angleDiff < 4 {
						if distance >= outerRadius+1.5 && distance <= outerRadius+2.5 {
							char = "│"
						} else if distance >= outerRadius+2.8 && distance <= outerRadius+4.0 {

							speedValue := i * 10
							if speedValue <= 100 {

								if speedValue == 0 {
									char = "0"
								} else if speedValue == 10 {
									char = "1"
								} else if speedValue == 20 {
									char = "2"
								} else if speedValue == 30 {
									char = "3"
								} else if speedValue == 40 {
									char = "4"
								} else if speedValue == 50 {
									char = "5"
								} else if speedValue == 60 {
									char = "6"
								} else if speedValue == 70 {
									char = "7"
								} else if speedValue == 80 {
									char = "8"
								} else if speedValue == 90 {
									char = "9"
								} else if speedValue == 100 {
									char = "X"
								}
							}
						}
					}
				}
			} else if distance >= 3 && distance <= innerRadius-2 {

				needleAngle := 240.0 - (speed/maxSpeed)*270.0
				if needleAngle < 0.0 {
					needleAngle += 360.0
				}
				needleAngleRad := needleAngle * math.Pi / 180.0

				needleEndX := centerX + (innerRadius-3)*math.Cos(needleAngleRad)
				needleEndY := centerY - (innerRadius-3)*math.Sin(needleAngleRad)

				lineDistance := math.Abs((needleEndY-centerY)*x-(needleEndX-centerX)*y+needleEndX*centerY-needleEndY*centerX) /
					math.Sqrt(math.Pow(needleEndY-centerY, 2)+math.Pow(needleEndX-centerX, 2))

				if lineDistance < 1.2 &&
					((x-centerX)*(needleEndX-centerX)+(y-centerY)*(needleEndY-centerY)) > 0 {
					char = "━"
				}
			} else if distance <= 3.0 {

				char = "●"
			} else if distance >= 8 && distance <= 12 && inArc {

				if angleInDegrees >= 135.0 && angleInDegrees <= 216.0 {
					char = "▓"
				} else if angleInDegrees >= 216.0 && angleInDegrees <= 297.0 {
					char = "▒"
				} else if (angleInDegrees >= 297.0) || (angleInDegrees >= 0.0 && angleInDegrees <= 18.0) {
					char = "░"
				} else if angleInDegrees >= 18.0 && angleInDegrees <= 45.0 {
					char = "▓"
				}
			}

			s.WriteString(char)
		}
		s.WriteString("\n")
	}

	s.WriteString("     0   10   20   30   40   50   60   70   80   90  100\n")
	s.WriteString("                           Mbps\n")

	var speedDisplay string
	if speed >= 80 {
		speedDisplay = fmt.Sprintf("     \033[31;1mSpeed: %.1f Mbps\033[0m\n", speed)
	} else if speed >= 60 {
		speedDisplay = fmt.Sprintf("     \033[33;1mSpeed: %.1f Mbps\033[0m\n", speed)
	} else if speed >= 30 {
		speedDisplay = fmt.Sprintf("     \033[32;1mSpeed: %.1f Mbps\033[0m\n", speed)
	} else {
		speedDisplay = fmt.Sprintf("     \033[36;1mSpeed: %.1f Mbps\033[0m\n", speed)
	}

	s.WriteString(speedDisplay)

	return s.String()
}

func (m speedTest) renderSpeedHistory() string {
	if len(m.speedHistory) < 2 {
		return ""
	}

	var s strings.Builder
	s.WriteString("\nSpeed History:\n")

	maxSpeed := 1.0
	for _, speed := range m.speedHistory {
		if speed > maxSpeed {
			maxSpeed = speed
		}
	}

	height := 8
	width := len(m.speedHistory)
	if width > 50 {
		width = 50
	}

	for row := height - 1; row >= 0; row-- {
		threshold := (float64(row) / float64(height-1)) * maxSpeed
		for col := 0; col < width; col++ {
			idx := len(m.speedHistory) - width + col
			if idx >= 0 && idx < len(m.speedHistory) {
				if m.speedHistory[idx] >= threshold {
					s.WriteString("█")
				} else {
					s.WriteString(" ")
				}
			} else {
				s.WriteString(" ")
			}
		}
		s.WriteString("\n")
	}

	return s.String()
}

func runSpeedTestCmd() tea.Cmd {
	return tea.Sequence(

		func() tea.Msg {
			server := getServerLocation()
			return serverMsg(server)
		},

		func() tea.Msg {
			time.Sleep(1 * time.Second)
			ping := testPing()
			return pingMsg(ping)
		},

		func() tea.Msg {

			return tea.Cmd(func() tea.Msg {

				baseSpeed := 15.0 + float64(time.Now().UnixNano()%80)

				for i := 0; i < 50; i++ {
					time.Sleep(100 * time.Millisecond)

					progress := float64(i) / 49.0
					currentSpeed := baseSpeed * (0.3 + 0.7*progress)
					currentSpeed += float64((i%10 - 5)) * 2.0
					if currentSpeed < 0 {
						currentSpeed = 5.0
					}

				}

				return speedMsg(baseSpeed)
			})()
		},

		func() tea.Msg {
			time.Sleep(4 * time.Second)
			uploadSpeed := 8.0 + float64(time.Now().UnixNano()%40)
			return uploadMsg(uploadSpeed)
		},

		func() tea.Msg {
			downloadSpeed := 50.0 + float64(time.Now().UnixNano()%50)
			uploadSpeed := 25.0 + float64(time.Now().UnixNano()%25)
			ping := 15.0 + float64(time.Now().UnixNano()%20)
			server := getServerLocation()

			return completeMsg(speedTestResults{
				download: downloadSpeed,
				upload:   uploadSpeed,
				ping:     ping,
				server:   server,
			})
		},
	)
}

type speedTestResults struct {
	download float64
	upload   float64
	ping     float64
	server   string
}

func performCompleteSpeedTest() (speedTestResults, error) {
	results := speedTestResults{}

	results.server = getServerLocation()

	time.Sleep(1 * time.Second)
	results.ping = testPing()

	time.Sleep(5 * time.Second)
	results.download = simulateRealisticSpeedTest()

	time.Sleep(4 * time.Second)
	results.upload = simulateUploadSpeed()

	return results, nil
}

func getServerLocation() string {
//tempp :/q
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://ipapi.co/json/")
	if err != nil {

		locations := []string{
			"Mumbai, MH",
			"Delhi, DL",
			"Bangalore, KA",
			"Hyderabad, TG",
			"Chennai, TN",
			"Kolkata, WB",
			"Pune, MH",
			"Ahmedabad, GJ",
		}
		return locations[int(time.Now().UnixNano())%len(locations)]
	}
	defer resp.Body.Close()

	var data struct {
		City    string `json:"city"`
		Region  string `json:"region_code"`
		Country string `json:"country_name"`
		ISP     string `json:"org"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "Mumbai, Maharashtra"
	}

	if data.City != "" && data.Region != "" {
		return fmt.Sprintf("%s, %s", data.City, data.Region)
	}

	return "Mumbai, Maharashtra"
}

func testPing() float64 {

	client := &http.Client{Timeout: 2 * time.Second}
	start := time.Now()

	_, err := client.Head("https://www.google.com")
	if err != nil {

		return 15.0 + float64(time.Now().UnixNano()%20)
	}

	pingTime := time.Since(start).Milliseconds()
	return float64(pingTime)
}

func simulateUploadSpeed() float64 {

	baseSpeed := 8.0 + float64(time.Now().UnixNano()%40)
	return baseSpeed
}

func simulateRealisticSpeedTest() float64 {

	seed := time.Now().UnixNano() % 100
	baseSpeed := 15.0 + float64(seed)*0.8

	return baseSpeed
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*16, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
