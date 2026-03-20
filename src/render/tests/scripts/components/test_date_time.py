from deephaven import ui

@ui.component
def test_date_time():
    date_val, set_date_val = ui.use_state(None)
    time_val, set_time_val = ui.use_state(None)

    return ui.flex(
        ui.date_field(label="Date Field", on_change=set_date_val),
        ui.text(f"Date field: {date_val}"),
        ui.date_picker(label="Date Picker", on_change=set_date_val),
        ui.text(f"Date picker: {date_val}"),
        ui.time_field(label="Time Field", on_change=set_time_val),
        ui.text(f"Time: {time_val}"),
        ui.calendar(aria_label="Event Calendar", default_value="2024-01-15"),
        direction="column", gap="size-100",
    )

date_time_widget = test_date_time()
