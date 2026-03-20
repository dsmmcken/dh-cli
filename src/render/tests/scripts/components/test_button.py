from deephaven import ui

@ui.component
def test_buttons():
    count, set_count = ui.use_state(0)
    toggled, set_toggled = ui.use_state(False)
    logic_variant, set_logic_variant = ui.use_state("or")

    return ui.flex(
        ui.heading(f"Button clicks: {count}"),
        ui.button("Primary", on_press=lambda: set_count(count + 1), variant="primary"),
        ui.button("Secondary", on_press=lambda: set_count(count + 2), variant="secondary"),
        ui.action_button("Action", on_press=lambda: set_count(count + 10)),
        ui.button_group(
            ui.button("A", on_press=lambda: set_count(count + 100)),
            ui.button("B", on_press=lambda: set_count(count + 200)),
        ),
        ui.toggle_button("Toggle", is_selected=toggled, on_change=set_toggled),
        ui.text(f"Toggle: {toggled}"),
        ui.logic_button(
            logic_variant,
            variant=logic_variant,
            on_press=lambda: set_logic_variant("and" if logic_variant == "or" else "or"),
        ),
        ui.text(f"Logic: {logic_variant}"),
        direction="column", gap="size-100",
    )

button_widget = test_buttons()
