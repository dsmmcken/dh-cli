from deephaven import ui

@ui.component
def err_explicit_throw():
    raise RuntimeError("This component intentionally throws an error")

err_explicit_throw_widget = err_explicit_throw()
