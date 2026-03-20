from deephaven import ui

@ui.component
def err_missing_prop():
    # Bug: checkbox_group needs children but on_change references wrong variable
    selected, set_selected = ui.use_state([])
    return ui.flex(
        ui.checkbox_group(
            label="Options",
            value=selected,
            on_change=set_selected,
            # Bug: no checkbox children provided
        ),
        direction="column",
    )

err_missing_prop_widget = err_missing_prop()
