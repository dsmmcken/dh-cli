from deephaven import ui

@ui.component
def err_stale_closure():
    count, set_count = ui.use_state(0)
    message, set_message = ui.use_state("")

    # Bug: the lambda captures count at definition time
    # Clicking "Show Count" always shows the initial count value
    # because set_message uses the stale count from closure
    def show_count():
        set_message(f"Count was: {count}")

    return ui.flex(
        ui.text(f"Count: {count}"),
        ui.text(f"Message: {message}"),
        ui.button("Increment", on_press=lambda: set_count(count + 1)),
        ui.button("Show Count", on_press=show_count),
        direction="column", gap="size-100",
    )

err_stale_closure_widget = err_stale_closure()
