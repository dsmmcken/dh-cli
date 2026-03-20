from deephaven import ui

@ui.component
def err_type_mismatch():
    count, set_count = ui.use_state(0)
    # Bug: trying to concatenate string and int without conversion
    label = "Count: " + count
    return ui.flex(
        ui.text(label),
        ui.button("Increment", on_press=lambda: set_count(count + 1)),
        direction="column",
    )

err_type_mismatch_widget = err_type_mismatch()
