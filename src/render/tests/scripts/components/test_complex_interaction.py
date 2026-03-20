from deephaven import ui

@ui.component
def test_complex():
    name, set_name = ui.use_state("")
    color, set_color = ui.use_state(None)
    agree, set_agree = ui.use_state(False)
    submitted, set_submitted = ui.use_state(False)
    result, set_result = ui.use_state("")

    def handle_submit():
        if name and color and agree:
            set_result(f"Submitted: {name} likes {color}")
            set_submitted(True)
        else:
            set_result("Please fill all fields and agree")

    def handle_reset():
        set_name("")
        set_color(None)
        set_agree(False)
        set_submitted(False)
        set_result("")

    return ui.flex(
        ui.heading("Registration Form"),
        ui.text_field(label="Your Name", value=name, on_change=set_name),
        ui.picker(
            "Red", "Green", "Blue", "Yellow",
            label="Favorite Color",
            selected_key=color,
            on_selection_change=set_color,
        ),
        ui.checkbox("I agree to terms", is_selected=agree, on_change=set_agree),
        ui.flex(
            ui.button("Submit", on_press=handle_submit, variant="primary"),
            ui.button("Reset", on_press=handle_reset, variant="secondary"),
            direction="row", gap="size-100",
        ),
        ui.text(f"Result: {result}"),
        ui.text(f"Submitted: {submitted}"),
        direction="column", gap="size-200",
    )

complex_widget = test_complex()
