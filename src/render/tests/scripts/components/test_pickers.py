from deephaven import ui

@ui.component
def test_pickers():
    picker_val, set_picker_val = ui.use_state(None)
    combo_val, set_combo_val = ui.use_state(None)

    return ui.flex(
        ui.picker(
            "Apple", "Banana", "Cherry", "Date",
            label="Fruit Picker",
            selected_key=picker_val,
            on_selection_change=set_picker_val,
        ),
        ui.text(f"Picked: {picker_val}"),
        ui.combo_box(
            ui.item("Red"),
            ui.item("Green"),
            ui.item("Blue"),
            label="Color Combo",
            selected_key=combo_val,
            on_change=set_combo_val,
        ),
        ui.text(f"Combo: {combo_val}"),
        direction="column", gap="size-100",
    )

pickers_widget = test_pickers()
