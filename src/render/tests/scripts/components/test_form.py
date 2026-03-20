from deephaven import ui

@ui.component
def test_form():
    submitted, set_submitted = ui.use_state(False)
    name_val, set_name_val = ui.use_state("")
    email_val, set_email_val = ui.use_state("")

    def handle_submit(data):
        set_submitted(True)

    return ui.flex(
        ui.form(
            ui.text_field(label="Name", value=name_val, on_change=set_name_val, is_required=True),
            ui.text_field(label="Email", value=email_val, on_change=set_email_val, is_required=True),
            ui.button("Submit", type="submit", variant="primary"),
            on_submit=handle_submit,
            validation_behavior="native",
        ),
        ui.text(f"Submitted: {submitted}"),
        ui.text(f"Name: {name_val}, Email: {email_val}"),
        direction="column", gap="size-100",
    )

form_widget = test_form()
