from deephaven import ui

@ui.component
def test_tabs():
    return ui.tabs(
        ui.tab(
            ui.flex(
                ui.text("Content of Tab 1"),
                ui.button("Tab 1 Button", on_press=lambda: None),
                direction="column",
            ),
            title="First Tab",
        ),
        ui.tab(
            ui.text("Content of Tab 2"),
            title="Second Tab",
        ),
        ui.tab(
            ui.text("Content of Tab 3"),
            title="Third Tab",
        ),
    )

tabs_widget = test_tabs()
