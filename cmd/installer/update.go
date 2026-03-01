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
		key := msg.String()

		// Universal control keys that should NEVER be passed to text inputs
		universalControlKeys := map[string]bool{
			"tab": true, "shift+tab": true, "enter": true, "esc": true,
			"ctrl+c": true,
		}

		// Step-specific control keys
		stepControlKeys := map[string]bool{}
		switch m.step {
		case stepWelcome, stepUninstallConfirm:
			stepControlKeys = map[string]bool{
				"up": true, "down": true, "k": true, "j": true, "q": true,
				"w": true, "W": true,
			}
		case stepPaths:
			stepControlKeys = map[string]bool{
				"+": true, "-": true,
			}
		case stepIntegrationsSonarr, stepIntegrationsRadarr, stepIntegrationsJellyfin:
			stepControlKeys = map[string]bool{
				"up": true, "down": true, "k": true, "j": true,
				"t": true, "T": true, "s": true, "S": true,
			}
		case stepIntegrationsAI:
			stepControlKeys = map[string]bool{
				"up": true, "down": true, "k": true, "j": true,
				"t": true, "T": true, "s": true, "S": true,
				"p": true, "P": true, "r": true, "R": true,
				"e": true, "E": true,
				" ": true,
			}
		case stepSystemService:
			stepControlKeys = map[string]bool{
				"up": true, "down": true, "k": true, "j": true,
			}
		case stepSystemWeb:
			stepControlKeys = map[string]bool{
				"up": true, "down": true, "k": true, "j": true,
				"e": true, "E": true,
			}
		case stepConfirm, stepComplete:
			stepControlKeys = map[string]bool{
				"q": true,
			}
		}

		// First, let step handlers process the key
		newModel, cmd := m.handleKeyPress(msg)

		// If not a control key for this step, pass to text input
		isControlKey := universalControlKeys[key] || stepControlKeys[key]
		if !isControlKey {
			mdl := newModel.(model)
			if len(mdl.inputs) > 0 {
				if mdl.step == stepSystemWeb && mdl.focusedInput == 2 {
					var inputCmd tea.Cmd
					mdl.inputs[0], inputCmd = mdl.inputs[0].Update(msg)
					return mdl, inputCmd
				}
				if mdl.step == stepIntegrationsJellyfin {
					inputIdx := mdl.focusedInput - 1
					if inputIdx >= 0 && inputIdx < len(mdl.inputs) {
						var inputCmd tea.Cmd
						mdl.inputs[inputIdx], inputCmd = mdl.inputs[inputIdx].Update(msg)
						return mdl, inputCmd
					}
				}
				if mdl.focusedInput < len(mdl.inputs) {
					if mdl.step == stepPaths || mdl.step == stepIntegrationsSonarr ||
						mdl.step == stepIntegrationsRadarr || mdl.step == stepIntegrationsAI ||
						mdl.step == stepSystemPermissions {
						var inputCmd tea.Cmd
						mdl.inputs[mdl.focusedInput], inputCmd = mdl.inputs[mdl.focusedInput].Update(msg)
						return mdl, inputCmd
					}
				}
			}
		}
		return newModel, cmd

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

	case scanStartMsg:
		m.scanCancel = msg.cancel
		return m, m.spinner.Tick

	case scanProgressMsg:
		m.scanProgress = msg.progress
		// Keep spinner ticking during scan
		return m, m.spinner.Tick

	case scanCompleteMsg:
		if msg.err != nil {
			m.errors = append(m.errors, msg.err.Error())
		}
		m.scanResult = msg.result
		m.scanStats = msg.stats
		m.scanCancel = nil

		// After scan, run post-scan tasks (systemd setup, start service)
		if len(m.postScanTasks) > 0 {
			m.tasks = m.postScanTasks
			m.postScanTasks = nil
			m.currentTaskIndex = 0
			m.tasks[0].status = statusRunning
			m.step = stepInstalling
			return m, executeTaskCmd(0, &m)
		}

		m.step = stepComplete
		return m, nil

	case arrIssuesMsg:
		if msg.err != nil {
			m.errors = append(m.errors, msg.err.Error())
		}
		if len(msg.issues) > 0 {
			m.arrIssues = msg.issues
			m.step = stepArrIssues
			m.arrIssuesChoice = 0
			return m, nil
		}
		// No issues found, proceed to scan
		m.step = stepScanning
		return m, m.runInitialScan()

	case arrFixMsg:
		// After fixing, proceed to scan
		m.arrIssues = nil
		m.step = stepScanning
		return m, m.runInitialScan()
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
	case stepIntegrationsJellyfin:
		return m.handleJellyfinKeys(key)
	case stepIntegrationsAI:
		return m.handleAIKeys(key)
	case stepSystemPermissions:
		return m.handlePermissionsKeys(key)
	case stepSystemService:
		return m.handleServiceKeys(key)
	case stepSystemWeb:
		return m.handleWebServiceKeys(key)
	case stepConfirm:
		return m.handleConfirmKeys(key)
	case stepUninstallConfirm:
		return m.handleUninstallConfirmKeys(key)
	case stepArrIssues:
		return m.handleArrIssuesKeys(key)
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

func (m model) handleJellyfinKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "tab":
		return m.nextJellyfinInput()
	case "shift+tab":
		return m.prevJellyfinInput()
	case "up", "k":
		if m.focusedInput == 0 {
			m.jellyfinEnabled = !m.jellyfinEnabled
		}
	case "down", "j":
		if m.focusedInput == 0 {
			m.jellyfinEnabled = !m.jellyfinEnabled
		}
	case "t", "T":
		if m.jellyfinEnabled {
			return m.testJellyfin()
		}
	case "s", "S":
		return m.nextStep()
	case "enter":
		m.saveJellyfinInputs()
		return m.nextStep()
	case "esc":
		return m.prevStep()
	}
	return m, nil
}

func (m model) handleAIKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "tab", "shift+tab":
		return m, nil
	case " ":
		if m.aiEnabled && m.aiState == aiStateReady && len(m.aiModels) > 0 {
			idx := m.aiModelIndex
			if idx >= 0 && idx < len(m.aiModels) {
				selectedModel := m.aiModels[idx]
				switch {
				case m.aiModel == selectedModel:
					// Deselect primary — promote fallback
					m.aiModel = m.aiFallbackModel
					m.aiFallbackModel = ""
					m.aiFallbackModelIndex = -1
				case m.aiFallbackModel == selectedModel:
					m.aiFallbackModel = ""
					m.aiFallbackModelIndex = -1
				case m.aiModel == "":
					m.aiModel = selectedModel
				case m.aiFallbackModel == "":
					m.aiFallbackModel = selectedModel
					m.aiFallbackModelIndex = idx
				}
			}
		} else {
			m.aiEnabled = !m.aiEnabled
			if m.aiEnabled {
				return m.detectOllama()
			}
		}
	case "e", "E":
		m.aiEnabled = !m.aiEnabled
		if m.aiEnabled {
			return m.detectOllama()
		}
	case "up", "k":
		if m.aiEnabled && m.aiState == aiStateReady && len(m.aiModels) > 0 {
			if m.aiModelIndex > 0 {
				m.aiModelIndex--
			}
		}
	case "down", "j":
		if m.aiEnabled && m.aiState == aiStateReady && len(m.aiModels) > 0 {
			if m.aiModelIndex < len(m.aiModels)-1 {
				m.aiModelIndex++
			}
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

func (m model) handleWebServiceKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "tab":
		m.focusedInput = (m.focusedInput + 1) % 3
	case "shift+tab":
		m.focusedInput = (m.focusedInput + 2) % 3
	case "up", "k":
		switch m.focusedInput {
		case 0:
			m.webEnabled = !m.webEnabled
		case 1:
			m.webStartNow = !m.webStartNow
		}
	case "down", "j":
		switch m.focusedInput {
		case 0:
			m.webEnabled = !m.webEnabled
		case 1:
			m.webStartNow = !m.webStartNow
		}
	case "e", "E":
		m.webEnabled = !m.webEnabled
	case "enter":
		m.saveWebInputs()
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

func (m model) handleArrIssuesKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.arrIssuesChoice > 0 {
			m.arrIssuesChoice--
		}
	case "down", "j":
		if m.arrIssuesChoice < 1 {
			m.arrIssuesChoice++
		}
	case "f", "F":
		// Fix issues
		return m, m.fixArrSettings()
	case "s", "S":
		// Skip / proceed without fixing
		m.step = stepScanning
		return m, m.runInitialScan()
	case "enter":
		if m.arrIssuesChoice == 0 {
			// Fix
			return m, m.fixArrSettings()
		}
		// Skip
		m.step = stepScanning
		return m, m.runInitialScan()
	}
	return m, nil
}

func (m model) handleUninstallConfirmKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.selectedOption > 0 {
			m.selectedOption--
		}
	case "down", "j":
		if m.selectedOption < 2 {
			m.selectedOption++
		}
	case "enter":
		// selectedOption: 0 = keep all, 1 = keep config/delete db, 2 = delete all
		switch m.selectedOption {
		case 0:
			m.keepDatabase = true
			m.keepConfig = true
		case 1:
			m.keepDatabase = false
			m.keepConfig = true
		case 2:
			m.keepDatabase = false
			m.keepConfig = false
		}
		return m.startInstallation()
	case "esc":
		// Go back to welcome
		m.uninstallMode = false
		m.step = stepWelcome
		m.selectedOption = 0
		return m, nil
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

func (m model) nextJellyfinInput() (tea.Model, tea.Cmd) {
	total := len(m.inputs) + 1 // include "Enable" as focusable row 0
	if total <= 1 {
		return m, nil
	}
	oldInputIdx := m.focusedInput - 1
	if oldInputIdx >= 0 && oldInputIdx < len(m.inputs) {
		m.inputs[oldInputIdx].Blur()
	}
	m.focusedInput = (m.focusedInput + 1) % total
	newInputIdx := m.focusedInput - 1
	if newInputIdx >= 0 && newInputIdx < len(m.inputs) {
		m.inputs[newInputIdx].Focus()
	}
	return m, nil
}

func (m model) prevJellyfinInput() (tea.Model, tea.Cmd) {
	total := len(m.inputs) + 1 // include "Enable" as focusable row 0
	if total <= 1 {
		return m, nil
	}
	oldInputIdx := m.focusedInput - 1
	if oldInputIdx >= 0 && oldInputIdx < len(m.inputs) {
		m.inputs[oldInputIdx].Blur()
	}
	m.focusedInput = (m.focusedInput + total - 1) % total
	newInputIdx := m.focusedInput - 1
	if newInputIdx >= 0 && newInputIdx < len(m.inputs) {
		m.inputs[newInputIdx].Focus()
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
	case stepIntegrationsJellyfin:
		m.initJellyfinInputs()
	case stepIntegrationsAI:
		m.initAIInputs()
	case stepSystemPermissions:
		m.initPermissionsInputs()
	case stepSystemWeb:
		m.initWebInputs()
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
			// Uninstall - go to confirmation screen
			m.uninstallMode = true
			m.step = stepUninstallConfirm
			m.selectedOption = 0 // Default to "keep config"
			return m, nil
		}
	} else {
		// Options: 0=Install, 1=Uninstall
		switch m.selectedOption {
		case 0:
			// Fresh install
			return m.nextStep()
		case 1:
			// Uninstall - go to confirmation screen
			m.uninstallMode = true
			m.step = stepUninstallConfirm
			m.selectedOption = 0 // Default to "keep config"
			return m, nil
		}
	}
	return m.nextStep()
}
