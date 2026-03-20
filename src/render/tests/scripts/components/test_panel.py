from deephaven import ui

@ui.component
def test_panel():
    return ui.panel(
        ui.flex(
            ui.heading("Panel Content"),
            ui.text("This is inside a panel."),
            ui.button("Panel Button", on_press=lambda: None),
            direction="column", gap="size-100",
        ),
        title="Test Panel",
    )

panel_widget = test_panel()
