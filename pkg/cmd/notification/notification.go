package notification

import (
	"net/http"

	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/spf13/cobra"
)

type NotificationOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
}

func NewCmdNotification(f *cmdutil.Factory) *cobra.Command {
	opts := &NotificationOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "notification",
		Short: "notification",
		Long:  "notification",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return notificationRun(opts)
		},
	}

	return cmd
}

func notificationRun(opts *NotificationOptions) error {
	app := tview.NewApplication()

	keymapTxt := "Movement: arrow keys\tQuit: q\tWeb: w\tSave: s\tUnsubscribe: u\tDone: d\tSelect: enter"
	keymap := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetText(keymapTxt)
	keymap.SetBorder(true).SetTitle("Help")

	notificationsList := NewList()
	notificationsList.SetBorder(true)
	notificationsList.SetSelectedFocusOnly(true)

	filterList := tview.NewList()
	filterList.AddItem("Inbox", "", 'i', func() { updateNotificationsList(app, notificationsList, "Inbox") })
	filterList.AddItem("Saved", "", 's', func() { updateNotificationsList(app, notificationsList, "Saved") })
	filterList.AddItem("Done", "", 'd', func() { updateNotificationsList(app, notificationsList, "Done") })
	filterList.SetBorder(true)

	filterList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRight {
			app.SetFocus(notificationsList)
			return nil
		}
		if event.Key() == tcell.KeyLeft {
			return nil
		}
		if event.Rune() == 'q' {
			app.Stop()
			return nil
		}
		return event
	})

	notificationsList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyLeft {
			app.SetFocus(filterList)
			return nil
		}
		if event.Key() == tcell.KeyRight {
			return nil
		}
		if event.Rune() == 'q' {
			app.Stop()
			return nil
		}
		return event
	})

	topRow := tview.NewFlex().
		AddItem(filterList, 0, 1, false).
		AddItem(notificationsList, 0, 4, false)

	bottomRow := tview.NewFlex().
		AddItem(keymap, 0, 1, false)

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(topRow, 0, 15, false).
		AddItem(bottomRow, 0, 1, false)

	if err := app.SetRoot(flex, true).SetFocus(filterList).Run(); err != nil {
		return err
	}

	return nil
}

func updateNotificationsList(app *tview.Application, list *List, filterType string) {
	list.SetTitle(filterType)
	list.Clear()
	switch filterType {
	case "Inbox":
		addInboxListItems(list)
	case "Saved":
		addSavedListItems(list)
	case "Done":
		addDoneListItems(list)
	}
	app.SetFocus(list)
}

func addInboxListItems(list *List) {
	list.AddItem("notification 1", "", 0, nil)
	list.AddItem("notification 2", "", 0, nil)
	list.AddItem("notification 3", "", 0, nil)
	list.AddItem("notification 4", "", 0, nil)
	list.AddItem("notification 5", "", 0, nil)
}

func addSavedListItems(list *List) {
	list.AddItem("saved notification 1", "", 0, nil)
	list.AddItem("saved notification 2", "", 0, nil)
	list.AddItem("saved notification 3", "", 0, nil)
	list.AddItem("saved notification 4", "", 0, nil)
	list.AddItem("saved notification 5", "", 0, nil)
}

func addDoneListItems(list *List) {
	list.AddItem("done notification 1", "", 0, nil)
	list.AddItem("done notification 2", "", 0, nil)
	list.AddItem("done notification 3", "", 0, nil)
	list.AddItem("done notification 4", "", 0, nil)
	list.AddItem("done notification 5", "", 0, nil)
}
