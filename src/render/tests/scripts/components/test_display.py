from deephaven import ui

@ui.component
def test_display():
    return ui.flex(
        ui.heading("Display Components", level=1),
        ui.text("Regular text content"),
        ui.badge("New", variant="positive"),
        ui.avatar(src="https://example.com/avatar.png", alt="User"),
        ui.icon("vsAccount"),
        ui.illustrated_message(
            ui.heading("No results"),
            ui.content("Try a different search term"),
        ),
        ui.labeled_value(label="CPU Usage", value="45%"),
        direction="column", gap="size-100",
    )

display_widget = test_display()
