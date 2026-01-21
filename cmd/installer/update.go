// cmd/installer/update.go
package main

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Calculate header height: 4 lines for ASCII art + 2 for padding
		headerHeight := 6
		if m.beams == nil {
			m.beams = NewBeamsTextEffect(msg.Width, headerHeight, asciiHeader)
		} else {
			m.beams.Resize(msg.Width, headerHeight)
		}
		return m, nil

	case tickMsg:
		if m.beams != nil {
			m.beams.Update()
		}
		if m.ticker != nil {
			m.ticker.Update()
		}
		return m, tickCmd()

	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case taskCompleteMsg:
		return m.handleTaskComplete(msg)

	case apiTestResultMsg:
		return m.handleAPITestResult(msg)

	case aiModelsMsg:
		return m.handleAIModels(msg)

	case aiPromptTestMsg:
		return m.handleAIPromptTest(msg)
	}

	if len(m.inputs) > 0 && m.focusedInput < len(m.inputs) {
		var cmd tea.Cmd
		m.inputs[m.focusedInput], cmd = m.inputs[m.focusedInput].Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "ctrl+c":
		if m.step != stepInstalling {
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
		return m, nil

	case "q":
		if m.step == stepComplete || m.step == stepWelcome {
			return m, tea.Quit
		}
	}

	switch m.step {
	case stepWelcome:
		return m.handleWelcomeKeys(key)
	case stepPaths:
		return m.handlePathsKeys(key)
	case stepIntegrationsSonarr:
		return m.handleSonarrKeys(key)
	case stepIntegrationsRadarr:
		return m.handleRadarrKeys(key)
	case stepIntegrationsAI:
		return m.handleAIKeys(key)
	case stepSystemPermissions:
		return m.handlePermissionsKeys(key)
	case stepSystemService:
		return m.handleServiceKeys(key)
	case stepConfirm:
		return m.handleConfirmKeys(key)
	case stepComplete:
		return m.handleCompleteKeys(key)
	}

	return m, nil
}

func (m model) handleWelcomeKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.selectedOption > 0 {
			m.selectedOption--
		}
	case "down", "j":
		maxOption := 1
		if m.existingDBDetected {
			maxOption = 2
		}
		if m.selectedOption < maxOption {
			m.selectedOption++
		}
	case "w", "W":
		if m.existingDBDetected {
			m.forceWizard = true
		}
	case "enter":
		return m.proceedFromWelcome()
	}
	return m, nil
}

func (m model) handlePathsKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "tab":
		return m.nextInput()
	case "shift+tab":
		return m.prevInput()
	case "+":
		return m.addWatchFolder()
	case "-":
		return m.removeWatchFolder()
	case "enter":
		m.savePathsInputs()
		return m.nextStep()
	case "esc":
		return m.prevStep()
	}
	return m, nil
}

func (m model) handleSonarrKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "tab":
		return m.nextInput()
	case "shift+tab":
		return m.prevInput()
	case "up", "k":
		if m.focusedInput == 0 {
			m.sonarrEnabled = !m.sonarrEnabled
		}
	case "down", "j":
		if m.focusedInput == 0 {
			m.sonarrEnabled = !m.sonarrEnabled
		}
	case "t", "T":
		if m.sonarrEnabled {
			return m.testSonarr()
		}
	case "s", "S":
		return m.nextStep()
	case "enter":
		m.saveSonarrInputs()
		return m.nextStep()
	case "esc":
		return m.prevStep()
	}
	return m, nil
}

func (m model) handleRadarrKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "tab":
		return m.nextInput()
	case "shift+tab":
		return m.prevInput()
	case "up", "k":
		if m.focusedInput == 0 {
			m.radarrEnabled = !m.radarrEnabled
		}
	case "down", "j":
		if m.focusedInput == 0 {
			m.radarrEnabled = !m.radarrEnabled
		}
	case "t", "T":
		if m.radarrEnabled {
			return m.testRadarr()
		}
	case "s", "S":
		return m.nextStep()
	case "enter":
		m.saveRadarrInputs()
		return m.nextStep()
	case "esc":
		return m.prevStep()
	}
	return m, nil
}

func (m model) handleAIKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "tab":
		return m.nextInput()
	case "shift+tab":
		return m.prevInput()
	case "up", "k":
		if m.focusedInput == 0 {
			m.aiEnabled = !m.aiEnabled
			if m.aiEnabled {
				return m.detectOllama()
			}
		} else if m.aiState == aiStateReady && m.aiModelIndex > 0 {
			m.aiModelIndex--
			m.aiModel = m.aiModels[m.aiModelIndex]
		}
	case "down", "j":
		if m.focusedInput == 0 {
			m.aiEnabled = !m.aiEnabled
			if m.aiEnabled {
				return m.detectOllama()
			}
		} else if m.aiState == aiStateReady && m.aiModelIndex < len(m.aiModels)-1 {
			m.aiModelIndex++
			m.aiModel = m.aiModels[m.aiModelIndex]
		}
	case "t", "T":
		if m.aiEnabled && m.aiState == aiStateReady {
			return m.testOllamaConnection()
		}
	case "p", "P":
		if m.aiEnabled && m.aiState == aiStateReady && m.aiModel != "" {
			return m.testOllamaPrompt()
		}
	case "r", "R":
		if m.aiEnabled {
			return m.detectOllama()
		}
	case "s", "S":
		return m.nextStep()
	case "enter":
		m.saveAIInputs()
		return m.nextStep()
	case "esc":
		return m.prevStep()
	}
	return m, nil
}

func (m model) handlePermissionsKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "tab":
		return m.nextInput()
	case "shift+tab":
		return m.prevInput()
	case "enter":
		m.savePermissionsInputs()
		return m.nextStep()
	case "esc":
		return m.prevStep()
	}
	return m, nil
}

func (m model) handleServiceKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "tab":
		m.focusedInput = (m.focusedInput + 1) % 3
	case "shift+tab":
		m.focusedInput = (m.focusedInput + 2) % 3
	case "up", "k":
		switch m.focusedInput {
		case 0:
			m.serviceEnabled = !m.serviceEnabled
		case 1:
			m.serviceStartNow = !m.serviceStartNow
		case 2:
			if m.scanFrequency > 0 {
				m.scanFrequency--
			}
		}
	case "down", "j":
		switch m.focusedInput {
		case 0:
			m.serviceEnabled = !m.serviceEnabled
		case 1:
			m.serviceStartNow = !m.serviceStartNow
		case 2:
			if m.scanFrequency < len(scanFrequencyOptions)-1 {
				m.scanFrequency++
			}
		}
	case "enter":
		return m.nextStep()
	case "esc":
		return m.prevStep()
	}
	return m, nil
}

func (m model) handleConfirmKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		return m.startInstallation()
	case "esc":
		return m.prevStep()
	}
	return m, nil
}

func (m model) handleCompleteKeys(key string) (tea.Model, tea.Cmd) {
	if key == "enter" || key == "q" {
		return m, tea.Quit
	}
	return m, nil
}

func (m model) nextInput() (tea.Model, tea.Cmd) {
	if len(m.inputs) > 0 {
		m.inputs[m.focusedInput].Blur()
		m.focusedInput = (m.focusedInput + 1) % len(m.inputs)
		m.inputs[m.focusedInput].Focus()
	}
	return m, nil
}

func (m model) prevInput() (tea.Model, tea.Cmd) {
	if len(m.inputs) > 0 {
		m.inputs[m.focusedInput].Blur()
		m.focusedInput = (m.focusedInput + len(m.inputs) - 1) % len(m.inputs)
		m.inputs[m.focusedInput].Focus()
	}
	return m, nil
}

func (m model) nextStep() (tea.Model, tea.Cmd) {
	m.step++
	m.focusedInput = 0
	m.initInputsForStep()
	return m, nil
}

func (m model) prevStep() (tea.Model, tea.Cmd) {
	if m.step > stepWelcome {
		m.step--
		m.focusedInput = 0
		m.initInputsForStep()
	}
	return m, nil
}

func (m *model) initInputsForStep() {
	m.inputs = []textinput.Model{}

	switch m.step {
	case stepPaths:
		m.initPathsInputs()
	case stepIntegrationsSonarr:
		m.initSonarrInputs()
	case stepIntegrationsRadarr:
		m.initRadarrInputs()
	case stepIntegrationsAI:
		m.initAIInputs()
	case stepSystemPermissions:
		m.initPermissionsInputs()
	}

	if len(m.inputs) > 0 {
		m.inputs[0].Focus()
	}
}

func (m model) proceedFromWelcome() (tea.Model, tea.Cmd) {
	// Handle selection based on whether existing DB is detected
	if m.existingDBDetected {
		// Options: 0=Update, 1=Install, 2=Uninstall
		switch m.selectedOption {
		case 0:
			// Update mode - skip wizard, just rebuild/reinstall binaries
			m.updateMode = true
			return m.startInstallation()
		case 1:
			// Fresh install
			return m.nextStep()
		case 2:
			// Uninstall
			m.uninstallMode = true
			return m.startInstallation()
		}
	} else {
		// Options: 0=Install, 1=Uninstall
		switch m.selectedOption {
		case 0:
			// Fresh install
			return m.nextStep()
		case 1:
			// Uninstall
			m.uninstallMode = true
			return m.startInstallation()
		}
	}
	return m.nextStep()
}
