from deephaven import ui

@ui.component
def test_inline_alert():
    return ui.flex(
        ui.inline_alert(ui.heading("Success"), ui.content("Operation completed."), variant="positive"),
        ui.inline_alert(ui.heading("Warning"), ui.content("Check your input."), variant="notice"),
        ui.inline_alert(ui.heading("Error"), ui.content("Something went wrong."), variant="negative"),
        ui.inline_alert(ui.heading("Info"), ui.content("FYI information."), variant="informative"),
        direction="column", gap="size-100",
    )

inline_alert_widget = test_inline_alert()
