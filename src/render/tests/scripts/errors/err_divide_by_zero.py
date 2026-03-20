from deephaven import ui

@ui.component
def err_divide_by_zero():
    total, set_total = ui.use_state(100)
    count, set_count = ui.use_state(0)
    # Bug: divides by count which starts at 0
    average = total / count
    return ui.flex(
        ui.text(f"Average: {average}"),
        ui.button("Add", on_press=lambda: set_count(count + 1)),
        direction="column",
    )

err_divide_by_zero_widget = err_divide_by_zero()
