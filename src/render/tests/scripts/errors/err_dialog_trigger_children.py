from deephaven import ui

@ui.component
def err_dialog_trigger_children():
    # Bug: dialog_trigger requires exactly 2 children (trigger + dialog).
    # Real Spectrum DialogTrigger throws:
    #   "DialogTrigger must have exactly 2 children"
    # Stubs silently render all children without validation.
    return ui.flex(
        ui.dialog_trigger(
            ui.action_button("Open"),
            ui.dialog(ui.heading("Title"), ui.content("Body")),
            ui.text("Extra child"),  # 3rd child — triggers Spectrum error
        ),
        direction="column",
    )

err_dialog_trigger_children_widget = err_dialog_trigger_children()
