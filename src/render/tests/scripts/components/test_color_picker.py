from deephaven import ui

@ui.component
def test_color_picker():
    color, set_color = ui.use_state("#ff0000")

    return ui.flex(
        ui.color_picker(label="Pick a color", value=color, on_change=set_color),
        ui.text(f"Color: {color}"),
        direction="column", gap="size-100",
    )

color_picker_widget = test_color_picker()
