from deephaven import ui

@ui.component
def test_list_view():
    selected, set_selected = ui.use_state([])

    return ui.flex(
        ui.list_view(
            ui.item("Item Alpha", key="alpha"),
            ui.item("Item Beta", key="beta"),
            ui.item("Item Gamma", key="gamma"),
            ui.item("Item Delta", key="delta"),
            selection_mode="MULTIPLE",
            selected_keys=selected,
            on_change=set_selected,
            aria_label="Test List",
        ),
        ui.text(f"Selected: {selected}"),
        direction="column", gap="size-100",
    )

list_view_widget = test_list_view()
