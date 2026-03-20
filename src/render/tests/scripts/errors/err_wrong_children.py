from deephaven import ui

@ui.component
def err_wrong_children():
    # Bug: passing a dict as children to a component that expects string/elements
    data = {"key1": "value1", "key2": "value2"}
    return ui.flex(
        ui.heading("Data Display"),
        ui.text(data),
        direction="column",
    )

err_wrong_children_widget = err_wrong_children()
