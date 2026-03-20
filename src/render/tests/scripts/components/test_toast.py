from deephaven import ui

@ui.component
def test_toast():
    last_toast, set_last_toast = ui.use_state("none")

    def show_info():
        ui.toast("Info toast shown", variant="positive")
        set_last_toast("info")

    def show_error():
        ui.toast("Error toast shown", variant="negative")
        set_last_toast("error")

    return ui.flex(
        ui.button("Show Info Toast", on_press=show_info),
        ui.button("Show Error Toast", on_press=show_error),
        ui.text(f"Last toast: {last_toast}"),
        direction="column", gap="size-100",
    )

toast_widget = test_toast()
