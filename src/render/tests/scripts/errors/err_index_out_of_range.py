from deephaven import ui

@ui.component
def err_index_out_of_range():
    items = ["a", "b", "c"]
    # Bug: off-by-one, accessing index 3 in a 3-element list
    return ui.text(f"Last item: {items[len(items)]}")

err_index_out_of_range_widget = err_index_out_of_range()
