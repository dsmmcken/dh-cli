from deephaven import ui

@ui.component
def err_key_error():
    config = {"name": "test", "version": "1.0"}
    # Bug: accessing a key that doesn't exist
    return ui.text(f"Author: {config['author']}")

err_key_error_widget = err_key_error()
