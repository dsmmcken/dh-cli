from deephaven import ui

@ui.component
def err_bad_callback():
    items, set_items = ui.use_state(["apple", "banana"])

    def add_item():
        # Bug: calling .append() returns None, then assigning None as state
        new_items = items.append("cherry")
        set_items(new_items)

    return ui.flex(
        ui.text(f"Items: {items}"),
        ui.button("Add Item", on_press=add_item),
        direction="column",
    )

err_bad_callback_widget = err_bad_callback()
