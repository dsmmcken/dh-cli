from deephaven import ui

@ui.component
def err_infinite_render():
    count, set_count = ui.use_state(0)
    # Bug: calling set_count during render (not in an event handler)
    # This triggers a re-render which triggers another set_count...
    if count < 5:
        set_count(count + 1)
    return ui.text(f"Count: {count}")

err_infinite_render_widget = err_infinite_render()
