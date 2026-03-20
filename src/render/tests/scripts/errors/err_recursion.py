from deephaven import ui

def recursive_format(data, depth=0):
    # Bug: no base case termination, infinite recursion
    return f"[{recursive_format(data, depth + 1)}]"

@ui.component
def err_recursion():
    return ui.text(recursive_format("hello"))

err_recursion_widget = err_recursion()
