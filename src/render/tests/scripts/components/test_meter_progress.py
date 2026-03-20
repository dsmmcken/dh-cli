from deephaven import ui

@ui.component
def test_meter_progress():
    progress, set_progress = ui.use_state(35)

    return ui.flex(
        ui.meter(value=75, label="Storage", variant="positive"),
        ui.progress_bar(value=progress, label="Upload Progress"),
        ui.progress_circle(value=60, aria_label="Loading"),
        ui.slider(label="Set Progress", value=progress, on_change=set_progress, min_value=0, max_value=100),
        ui.text(f"Progress: {progress}%"),
        direction="column", gap="size-100",
    )

meter_progress_widget = test_meter_progress()
