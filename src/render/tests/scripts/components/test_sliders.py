from deephaven import ui

@ui.component
def test_sliders():
    slider_val, set_slider_val = ui.use_state(50)
    range_val, set_range_val = ui.use_state({"start": 20, "end": 80})

    return ui.flex(
        ui.slider(label="Volume", value=slider_val, on_change=set_slider_val, min_value=0, max_value=100),
        ui.text(f"Slider: {slider_val}"),
        ui.range_slider(label="Price Range", value=range_val, on_change=set_range_val, min_value=0, max_value=100),
        ui.text(f"Range: {range_val}"),
        direction="column", gap="size-100",
    )

sliders_widget = test_sliders()
