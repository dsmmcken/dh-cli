from deephaven import ui

@ui.component
def test_disclosure():
    is_expanded, set_is_expanded = ui.use_state(False)

    return ui.flex(
        ui.disclosure(
            title="Click to expand",
            panel="Hidden content revealed!",
            is_expanded=is_expanded,
            on_expanded_change=lambda: set_is_expanded(not is_expanded),
        ),
        ui.text(f"Expanded: {is_expanded}"),
        direction="column", gap="size-100",
    )

disclosure_widget = test_disclosure()
