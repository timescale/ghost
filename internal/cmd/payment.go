package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

func buildPaymentInteractiveCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "payment",
		Short:             "Manage payment methods",
		Long:              `Manage payment methods for your Ghost space. Opens an interactive menu when run from a terminal, or lists payment methods otherwise.`,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.OutOrStdout()) {
				// Fall back to list subcommand when not in a terminal
				listCmd, _, err := cmd.Find([]string{"list"})
				if err != nil {
					return err
				}
				return listCmd.RunE(listCmd, nil)
			}

			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			// Fetch payment methods
			resp, err := client.ListPaymentMethodsWithResponse(cmd.Context(), projectID)
			if err != nil {
				return fmt.Errorf("failed to list payment methods: %w", err)
			}

			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
			}

			if resp.JSON200 == nil {
				return errors.New("empty response from API")
			}

			model := newPaymentInteractiveModel(
				cmd.Context(),
				cfg.APIURL,
				client,
				projectID,
				resp.JSON200.PaymentMethods,
			)

			program := tea.NewProgram(model, tea.WithInput(cmd.InOrStdin()), tea.WithOutput(cmd.OutOrStdout()))
			finalModel, err := program.Run()
			if err != nil {
				return fmt.Errorf("failed to run interactive payment menu: %w", err)
			}

			result := finalModel.(paymentInteractiveModel)
			if result.err != nil {
				return result.err
			}

			return nil
		},
	}

	cmd.AddCommand(buildPaymentListCmd(app))
	cmd.AddCommand(buildPaymentAddCmd(app))
	cmd.AddCommand(buildPaymentDeleteCmd(app))
	cmd.AddCommand(buildPaymentPrimaryCmd(app))
	cmd.AddCommand(buildPaymentUndeleteCmd(app))

	return cmd
}

// -- Styles --

var (
	primaryBadge = lipgloss.NewStyle().Foreground(lipgloss.Green)

	deletionBadge = lipgloss.NewStyle().Foreground(lipgloss.Red)

	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Green)

	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Red)

	helpKeyStyle = lipgloss.NewStyle().Foreground(lipgloss.Blue)

	helpDescStyle = lipgloss.NewStyle().Foreground(lipgloss.White)

	titleStyle = lipgloss.NewStyle().Bold(true)

	subtleStyle = lipgloss.NewStyle().Faint(true)
)

// -- Messages --

type paymentMethodsRefreshedMsg struct {
	methods []api.PaymentMethod
}

type paymentActionResultMsg struct {
	message string
	err     error
}

type paymentBrowserOpenedMsg struct{}

// -- Model --

type paymentInteractiveModel struct {
	ctx         context.Context
	apiURL      string
	client      api.ClientWithResponsesInterface
	projectID   string
	methods     []api.PaymentMethod
	cursor      int
	status      string // success message
	errMsg      string // error message
	loading     bool   // true when an API action is in progress
	showMenu    bool   // true when showing the action menu for a selected card
	menuIdx     int    // cursor within the action menu
	showConfirm bool   // true when showing the delete confirmation prompt
	confirmIdx  int    // cursor within the confirmation prompt (0=Yes, 1=No)
	err         error  // fatal error that causes exit
}

func newPaymentInteractiveModel(
	ctx context.Context,
	apiURL string,
	client api.ClientWithResponsesInterface,
	projectID string,
	methods []api.PaymentMethod,
) paymentInteractiveModel {
	return paymentInteractiveModel{
		ctx:       ctx,
		apiURL:    apiURL,
		client:    client,
		projectID: projectID,
		methods:   methods,
	}
}

func (m paymentInteractiveModel) Init() tea.Cmd {
	return nil
}

func (m paymentInteractiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case paymentMethodsRefreshedMsg:
		m.loading = false
		m.methods = msg.methods
		// Clamp cursor in case items were removed
		if m.cursor >= len(m.methods) {
			m.cursor = max(0, len(m.methods)-1)
		}
		return m, nil

	case paymentActionResultMsg:
		if msg.err != nil {
			m.loading = false
			m.errMsg = msg.err.Error()
			m.status = ""
			return m, nil
		}
		m.status = msg.message
		m.errMsg = ""
		// Refresh the list after a successful action
		return m, m.refreshMethods()

	case paymentBrowserOpenedMsg:
		m.loading = false
		m.status = "Opened browser to add payment method. Press 'r' to refresh after completing the form."
		m.errMsg = ""
		return m, nil

	case tea.KeyPressMsg:
		// Always allow quitting, even during loading
		switch msg.String() {
		case "ctrl+c", "ctrl+d":
			return m, tea.Quit
		}

		// If loading, ignore all other input
		if m.loading {
			return m, nil
		}

		// If showing delete confirmation, handle confirmation keys
		if m.showConfirm {
			return m.updateConfirm(msg)
		}

		// If in the action menu, handle menu-specific keys
		if m.showMenu {
			return m.updateMenu(msg)
		}

		return m.updateMain(msg)
	}

	return m, nil
}

func (m paymentInteractiveModel) updateMain(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "ctrl+d", "esc", "q":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		m.status = ""
		m.errMsg = ""

	case "down", "j":
		if m.cursor < len(m.methods)-1 {
			m.cursor++
		}
		m.status = ""
		m.errMsg = ""

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(msg.String()[0]-'0') - 1
		if idx >= 0 && idx < len(m.methods) {
			m.cursor = idx
		}
		m.status = ""
		m.errMsg = ""

	case "enter":
		if len(m.methods) > 0 {
			m.showMenu = true
			m.menuIdx = 0
			m.status = ""
			m.errMsg = ""
		}

	case "p":
		if len(m.methods) > 0 {
			pm := m.methods[m.cursor]
			if !pm.Primary {
				return m.startAction(m.setPrimary(pm))
			}
		}

	case "d", "delete":
		if len(m.methods) > 0 {
			pm := m.methods[m.cursor]
			if !pm.PendingDeletion {
				m.showConfirm = true
				m.confirmIdx = 1 // default to "No" for safety
				m.status = ""
				m.errMsg = ""
			}
		}

	case "c":
		if len(m.methods) > 0 {
			pm := m.methods[m.cursor]
			if pm.PendingDeletion {
				return m.startAction(m.cancelDeletion(pm))
			}
		}

	case "a":
		return m.startAction(m.addPaymentMethod())

	case "r":
		return m.startAction(m.refreshMethods())
	}

	return m, nil
}

func (m paymentInteractiveModel) updateMenu(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	actions := m.menuActions()

	switch msg.String() {
	case "ctrl+c", "ctrl+d":
		return m, tea.Quit

	case "esc", "q":
		m.showMenu = false
		return m, nil

	case "up", "k":
		if m.menuIdx > 0 {
			m.menuIdx--
		}

	case "down", "j":
		if m.menuIdx < len(actions)-1 {
			m.menuIdx++
		}

	case "enter":
		if m.menuIdx < len(actions) {
			action := actions[m.menuIdx]
			m.showMenu = false
			if action.needsConfirm {
				m.showConfirm = true
				m.confirmIdx = 1 // default to "No" for safety
				m.status = ""
				m.errMsg = ""
				return m, nil
			}
			return m.startAction(action.cmd)
		}
	}

	return m, nil
}

func (m paymentInteractiveModel) updateConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "ctrl+d":
		return m, tea.Quit

	case "esc", "q", "n":
		m.showConfirm = false
		return m, nil

	case "up", "k":
		if m.confirmIdx > 0 {
			m.confirmIdx--
		}

	case "down", "j":
		if m.confirmIdx < 1 {
			m.confirmIdx++
		}

	case "y":
		m.showConfirm = false
		pm := m.methods[m.cursor]
		return m.startAction(m.deletePaymentMethod(pm))

	case "enter":
		m.showConfirm = false
		if m.confirmIdx == 0 {
			pm := m.methods[m.cursor]
			return m.startAction(m.deletePaymentMethod(pm))
		}
		return m, nil
	}

	return m, nil
}

type menuAction struct {
	label        string
	cmd          tea.Cmd
	needsConfirm bool
}

func (m paymentInteractiveModel) menuActions() []menuAction {
	if len(m.methods) == 0 {
		return nil
	}

	pm := m.methods[m.cursor]
	var actions []menuAction

	if !pm.Primary {
		actions = append(actions, menuAction{
			label: "Set as primary",
			cmd:   m.setPrimary(pm),
		})
	}
	if !pm.PendingDeletion {
		actions = append(actions, menuAction{
			label:        "Delete",
			cmd:          m.deletePaymentMethod(pm),
			needsConfirm: true,
		})
	}
	if pm.PendingDeletion {
		actions = append(actions, menuAction{
			label: "Cancel pending deletion",
			cmd:   m.cancelDeletion(pm),
		})
	}

	return actions
}

func (m paymentInteractiveModel) startAction(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.loading = true
	m.status = ""
	m.errMsg = ""
	return m, cmd
}

// -- API commands --

// apiError returns an error message from an API error response, falling back
// to the HTTP status code if no structured error is available.
func apiError(prefix string, statusCode int, jsonDefault *api.Error) error {
	if jsonDefault != nil {
		return fmt.Errorf("%s: %s", prefix, jsonDefault.Error())
	}
	return fmt.Errorf("%s: HTTP %d", prefix, statusCode)
}

func (m paymentInteractiveModel) refreshMethods() tea.Cmd {
	return func() tea.Msg {
		resp, err := m.client.ListPaymentMethodsWithResponse(m.ctx, m.projectID)
		if err != nil {
			return paymentActionResultMsg{err: fmt.Errorf("failed to refresh: %w", err)}
		}
		if resp.StatusCode() != http.StatusOK {
			return paymentActionResultMsg{err: apiError("failed to refresh", resp.StatusCode(), resp.JSONDefault)}
		}
		if resp.JSON200 == nil {
			return paymentActionResultMsg{err: errors.New("empty response from API")}
		}
		return paymentMethodsRefreshedMsg{methods: resp.JSON200.PaymentMethods}
	}
}

func (m paymentInteractiveModel) setPrimary(pm api.PaymentMethod) tea.Cmd {
	return func() tea.Msg {
		resp, err := m.client.SetPaymentMethodPrimaryWithResponse(m.ctx, m.projectID, pm.Id)
		if err != nil {
			return paymentActionResultMsg{err: fmt.Errorf("failed to set primary: %w", err)}
		}
		if resp.StatusCode() != http.StatusNoContent {
			return paymentActionResultMsg{err: apiError("failed to set primary", resp.StatusCode(), resp.JSONDefault)}
		}
		return paymentActionResultMsg{
			message: fmt.Sprintf("%s ending in %s is now your primary payment method.", pm.Brand, pm.Last4),
		}
	}
}

func (m paymentInteractiveModel) deletePaymentMethod(pm api.PaymentMethod) tea.Cmd {
	return func() tea.Msg {
		resp, err := m.client.DeletePaymentMethodWithResponse(m.ctx, m.projectID, pm.Id)
		if err != nil {
			return paymentActionResultMsg{err: fmt.Errorf("failed to delete: %w", err)}
		}
		if resp.StatusCode() != http.StatusNoContent {
			return paymentActionResultMsg{err: apiError("failed to delete", resp.StatusCode(), resp.JSONDefault)}
		}
		return paymentActionResultMsg{
			message: fmt.Sprintf("Deleted %s ending in %s.", pm.Brand, pm.Last4),
		}
	}
}

func (m paymentInteractiveModel) cancelDeletion(pm api.PaymentMethod) tea.Cmd {
	return func() tea.Msg {
		resp, err := m.client.CancelPaymentMethodDeletionWithResponse(m.ctx, m.projectID, pm.Id)
		if err != nil {
			return paymentActionResultMsg{err: fmt.Errorf("failed to cancel deletion: %w", err)}
		}
		if resp.StatusCode() != http.StatusNoContent {
			return paymentActionResultMsg{err: apiError("failed to cancel deletion", resp.StatusCode(), resp.JSONDefault)}
		}
		return paymentActionResultMsg{
			message: fmt.Sprintf("Cancelled pending deletion for %s ending in %s.", pm.Brand, pm.Last4),
		}
	}
}

func (m paymentInteractiveModel) addPaymentMethod() tea.Cmd {
	return func() tea.Msg {
		resp, err := m.client.CreatePaymentMethodSetupWithResponse(m.ctx, m.projectID)
		if err != nil {
			return paymentActionResultMsg{err: fmt.Errorf("failed to create payment setup: %w", err)}
		}
		if resp.StatusCode() != http.StatusOK {
			return paymentActionResultMsg{err: apiError("failed to create payment setup", resp.StatusCode(), resp.JSONDefault)}
		}
		if resp.JSON200 == nil {
			return paymentActionResultMsg{err: errors.New("empty response from API")}
		}

		paymentURL := m.apiURL + resp.JSON200.PaymentUrl
		if err := common.OpenBrowser(paymentURL); err != nil {
			return paymentActionResultMsg{
				err: fmt.Errorf("could not open browser; visit %s manually", paymentURL),
			}
		}

		return paymentBrowserOpenedMsg{}
	}
}

// -- View --

func (m paymentInteractiveModel) View() tea.View {
	var s strings.Builder

	s.WriteString(titleStyle.Render("Payment Methods"))
	s.WriteString("\n\n")

	if len(m.methods) == 0 {
		s.WriteString("No payment methods on file.\n")
	} else {
		for i, pm := range m.methods {
			selected := i == m.cursor
			s.WriteString(m.renderCard(i, pm, selected))
			s.WriteString("\n")
		}
	}

	// Status/error messages
	if m.loading {
		s.WriteString(subtleStyle.Render("Working...") + "\n")
	} else if m.errMsg != "" {
		s.WriteString(errorStyle.Render("Error: "+m.errMsg) + "\n")
	} else if m.status != "" {
		s.WriteString(statusStyle.Render(m.status) + "\n")
	}

	// Action menu / confirmation overlay
	if m.showConfirm {
		s.WriteString("\n")
		s.WriteString(m.renderConfirm())
	} else if m.showMenu {
		s.WriteString("\n")
		s.WriteString(m.renderMenu())
	} else {
		s.WriteString("\n")
		s.WriteString(m.renderHelp())
	}

	return tea.NewView(s.String())
}

func (m paymentInteractiveModel) renderCard(index int, pm api.PaymentMethod, selected bool) string {
	indicator := "  "
	if selected {
		indicator = "> "
	}

	// First line: indicator, number, brand/last4, badges
	brandLine := fmt.Sprintf("%s ending in %s", pm.Brand, pm.Last4)

	var badges []string
	if pm.Primary {
		badges = append(badges, primaryBadge.Render("PRIMARY"))
	}
	if pm.PendingDeletion {
		badges = append(badges, deletionBadge.Render("PENDING DELETION"))
	}

	var s strings.Builder
	s.WriteString(fmt.Sprintf("%s%d. %s", indicator, index+1, brandLine))
	if len(badges) > 0 {
		s.WriteString("  " + strings.Join(badges, " "))
	}

	// Indent detail lines to align with the text after "N. "
	padding := strings.Repeat(" ", len(indicator)+len(fmt.Sprintf("%d. ", index+1)))
	s.WriteString("\n" + padding + subtleStyle.Render(fmt.Sprintf("Expires %02d/%d  %s", pm.ExpMonth, pm.ExpYear, pm.Id)))

	return s.String()
}

func (m paymentInteractiveModel) renderMenu() string {
	actions := m.menuActions()
	if len(actions) == 0 {
		return helpDescStyle.Render("No actions available for this payment method. Press esc to go back.")
	}

	var s strings.Builder
	pm := m.methods[m.cursor]
	s.WriteString(fmt.Sprintf("Actions for %s ending in %s:\n\n", pm.Brand, pm.Last4))

	for i, action := range actions {
		cursor := "  "
		if i == m.menuIdx {
			cursor = "> "
		}
		s.WriteString(cursor + action.label + "\n")
	}

	s.WriteString("\n" + helpDescStyle.Render("enter select  esc back"))

	return s.String()
}

func (m paymentInteractiveModel) renderConfirm() string {
	var s strings.Builder
	pm := m.methods[m.cursor]
	s.WriteString(fmt.Sprintf("Delete %s ending in %s?\n\n", pm.Brand, pm.Last4))

	options := []string{"Yes, delete", "No, cancel"}
	for i, opt := range options {
		cursor := "  "
		if i == m.confirmIdx {
			cursor = "> "
		}
		s.WriteString(cursor + opt + "\n")
	}

	s.WriteString("\n" + helpDescStyle.Render("enter select  y confirm  esc cancel"))

	return s.String()
}

func (m paymentInteractiveModel) renderHelp() string {
	var parts []string

	parts = append(parts, helpEntry("↑/↓", "navigate"))

	if len(m.methods) > 0 {
		pm := m.methods[m.cursor]

		parts = append(parts, helpEntry("enter", "actions"))

		if !pm.Primary {
			parts = append(parts, helpEntry("p", "set primary"))
		}
		if !pm.PendingDeletion {
			parts = append(parts, helpEntry("d", "delete"))
		}
		if pm.PendingDeletion {
			parts = append(parts, helpEntry("c", "cancel deletion"))
		}
	}

	parts = append(parts, helpEntry("a", "add new"))
	parts = append(parts, helpEntry("r", "refresh"))
	parts = append(parts, helpEntry("esc", "quit"))

	return strings.Join(parts, helpDescStyle.Render("  "))
}

func helpEntry(key, desc string) string {
	return helpKeyStyle.Render(key) + " " + helpDescStyle.Render(desc)
}
