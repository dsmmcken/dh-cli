from deephaven import ui

@ui.component
def test_contextual_help():
    return ui.flex(
        ui.contextual_help(
            ui.heading("What is this?"),
            ui.content("This is a help tooltip that provides additional context."),
        ),
        ui.text("Hover the help icon above"),
        direction="column", gap="size-100",
    )

contextual_help_widget = test_contextual_help()
