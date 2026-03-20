from deephaven import ui

@ui.component
def test_menu():
    selected, set_selected = ui.use_state("none")

    return ui.flex(
        ui.action_menu(
            "Cut", "Copy", "Paste", "Delete",
            on_action=set_selected,
        ),
        ui.text(f"Menu action: {selected}"),
        ui.menu_trigger(
            ui.action_button("File Menu"),
            ui.menu(
                ui.item("New", key="new"),
                ui.item("Open", key="open"),
                ui.item("Save", key="save"),
                on_action=set_selected,
            ),
        ),
        direction="column", gap="size-100",
    )

menu_widget = test_menu()
