from deephaven import ui

@ui.component
def test_fragment():
    count, set_count = ui.use_state(0)

    return ui.fragment(
        ui.heading("Fragment Test"),
        ui.text(f"Count: {count}"),
        ui.button("Increment", on_press=lambda: set_count(count + 1)),
    )

fragment_widget = test_fragment()
