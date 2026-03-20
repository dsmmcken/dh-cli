from deephaven import ui

@ui.component
def test_dialog():
    is_open, set_is_open = ui.use_state(False)
    action_taken, set_action_taken = ui.use_state("none")

    def open_dialog():
        set_is_open(True)

    def confirm():
        set_action_taken("confirmed")
        set_is_open(False)

    def cancel():
        set_action_taken("cancelled")
        set_is_open(False)

    return ui.flex(
        ui.action_button("Open Dialog", on_press=open_dialog),
        ui.text(f"Action: {action_taken}"),
        ui.dialog_trigger(
            ui.action_button("Dismissable Dialog"),
            ui.dialog(
                ui.heading("Info"),
                ui.content("This dialog is dismissable."),
            ),
            is_dismissable=True,
        ),
        direction="column", gap="size-100",
    )

dialog_widget = test_dialog()
