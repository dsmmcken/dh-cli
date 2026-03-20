from deephaven import ui

@ui.component
def err_mutation():
    items, set_items = ui.use_state(["initial"])

    def add_item():
        # Bug: mutating the list directly instead of creating a new one
        # React won't detect the change because the reference is the same
        items.append("new item")
        set_items(items)

    return ui.flex(
        ui.text(f"Items ({len(items)}): {', '.join(items)}"),
        ui.button("Add", on_press=add_item),
        direction="column",
    )

err_mutation_widget = err_mutation()
