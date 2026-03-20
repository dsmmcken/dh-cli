from deephaven import ui

@ui.component
def test_checkboxes():
    checked, set_checked = ui.use_state(False)
    group_val, set_group_val = ui.use_state([])
    switched, set_switched = ui.use_state(False)
    radio_val, set_radio_val = ui.use_state("A")

    return ui.flex(
        ui.checkbox("Accept terms", is_selected=checked, on_change=set_checked),
        ui.text(f"Checked: {checked}"),
        ui.checkbox_group(
            ui.checkbox("Option 1", value="opt1"),
            ui.checkbox("Option 2", value="opt2"),
            ui.checkbox("Option 3", value="opt3"),
            label="Options",
            value=group_val,
            on_change=set_group_val,
        ),
        ui.text(f"Group: {group_val}"),
        ui.switch("Dark mode", is_selected=switched, on_change=set_switched),
        ui.text(f"Switched: {switched}"),
        ui.radio_group(
            ui.radio("Option A", value="A"),
            ui.radio("Option B", value="B"),
            ui.radio("Option C", value="C"),
            label="Radio Group",
            value=radio_val,
            on_change=set_radio_val,
        ),
        ui.text(f"Radio: {radio_val}"),
        direction="column", gap="size-100",
    )

checkboxes_widget = test_checkboxes()
