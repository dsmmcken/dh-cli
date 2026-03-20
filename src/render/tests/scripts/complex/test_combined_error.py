from deephaven import ui

@ui.component
def error_component():
    should_error, set_should_error = ui.use_state(False)

    if should_error:
        raise ValueError("Intentional error for testing")

    return ui.flex(
        ui.text("This component is working"),
        ui.button("Trigger Error", on_press=lambda: set_should_error(True)),
        direction="column",
    )

combined_error_widget = error_component()
