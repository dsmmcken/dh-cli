from deephaven import ui

@ui.component
def err_none_access():
    data, set_data = ui.use_state(None)
    # Bug: accessing .upper() on None - common mistake when data hasn't loaded yet
    return ui.text(f"Value: {data.upper()}")

err_none_access_widget = err_none_access()
