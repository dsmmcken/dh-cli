from deephaven import ui, new_table
from deephaven.column import string_col, int_col, double_col
import deephaven.plot.express as dx

# Sample data for table and figure
sample_table = new_table([
    string_col("Name", ["Alice", "Bob", "Charlie"]),
    int_col("Age", [30, 25, 35]),
    double_col("Score", [95.5, 87.3, 91.8]),
])

sample_figure = dx.scatter(sample_table, x="Age", y="Score")


@ui.component
def all_components():
    # -- State --
    text_val, set_text_val = ui.use_state("hello")
    area_val, set_area_val = ui.use_state("multi\nline")
    search_val, set_search_val = ui.use_state("")
    num_val, set_num_val = ui.use_state(42)
    slider_val, set_slider_val = ui.use_state(50)
    range_val, set_range_val = ui.use_state({"start": 20, "end": 80})
    checked, set_checked = ui.use_state(False)
    group_val, set_group_val = ui.use_state([])
    switched, set_switched = ui.use_state(False)
    radio_val, set_radio_val = ui.use_state("a")
    picker_val, set_picker_val = ui.use_state(None)
    combo_val, set_combo_val = ui.use_state(None)
    color_val, set_color_val = ui.use_state("#ff0000")
    toggled, set_toggled = ui.use_state(False)
    logic_val, set_logic_val = ui.use_state("and")
    tags, set_tags = ui.use_state(["one", "two", "three"])
    expanded, set_expanded = ui.use_state(False)
    date_val, set_date_val = ui.use_state(None)
    time_val, set_time_val = ui.use_state(None)
    menu_action, set_menu_action = ui.use_state("none")
    list_sel, set_list_sel = ui.use_state([])

    return ui.flex(
        # ── Text & Display ──
        ui.heading("All Components"),
        ui.text("Plain text"),
        ui.badge("Badge", variant="positive"),
        ui.avatar(src="https://example.com/avatar.png", alt="User Avatar"),
        ui.icon("account"),
        ui.labeled_value(label="Score", value="95.5"),
        ui.divider(),
        ui.markdown("**Bold** and *italic*"),
        ui.image(src="https://example.com/img.png", alt="Sample Image"),

        # ── Text Inputs ──
        ui.text_field(label="Text Field", value=text_val, on_change=set_text_val),
        ui.text_area(label="Text Area", value=area_val, on_change=set_area_val),
        ui.search_field(label="Search", value=search_val, on_change=set_search_val),
        ui.number_field(label="Number", value=num_val, on_change=set_num_val),

        # ── Buttons ──
        ui.button("Primary", variant="primary"),
        ui.button("Secondary", variant="secondary"),
        ui.action_button("Action"),
        ui.toggle_button("Toggle", is_selected=toggled, on_change=set_toggled),
        ui.logic_button(logic_val, variant=logic_val,
            on_press=lambda: set_logic_val("or" if logic_val == "and" else "and")),
        ui.button_group(
            ui.button("Btn A"),
            ui.button("Btn B"),
        ),
        ui.action_group("Edit", "Delete", on_action=lambda k: None),

        # ── Checkboxes, Switch, Radio ──
        ui.checkbox("Accept terms", is_selected=checked, on_change=set_checked),
        ui.checkbox_group(
            ui.checkbox("Opt 1", value="1"),
            ui.checkbox("Opt 2", value="2"),
            label="Checkbox Group",
            value=group_val,
            on_change=set_group_val,
        ),
        ui.switch("Dark mode", is_selected=switched, on_change=set_switched),
        ui.radio_group(
            ui.radio("Alpha", value="a"),
            ui.radio("Beta", value="b"),
            label="Radio Group",
            value=radio_val,
            on_change=set_radio_val,
        ),

        # ── Pickers & ComboBox ──
        ui.picker(
            "Apple", "Banana", "Cherry",
            label="Fruit Picker",
            selected_key=picker_val,
            on_selection_change=set_picker_val,
        ),
        ui.combo_box(
            ui.item("Red"), ui.item("Green"), ui.item("Blue"),
            label="Color Combo",
            selected_key=combo_val,
            on_change=set_combo_val,
        ),
        ui.color_picker(label="Color Picker", value=color_val, on_change=set_color_val),

        # ── Sliders ──
        ui.slider(label="Volume", value=slider_val, on_change=set_slider_val,
                  min_value=0, max_value=100),
        ui.range_slider(label="Range", value=range_val, on_change=set_range_val,
                        min_value=0, max_value=100),

        # ── Progress & Meters ──
        ui.meter(value=75, label="Meter", variant="positive"),
        ui.progress_bar(value=60, label="Progress"),
        ui.progress_circle(value=80, aria_label="Loading"),

        # ── Date & Time ──
        ui.date_field(label="Date Field", on_change=set_date_val),
        ui.date_picker(label="Date Picker", on_change=set_date_val),
        ui.date_range_picker(label="Date Range Picker"),
        ui.time_field(label="Time Field", on_change=set_time_val),
        ui.calendar(aria_label="Calendar", default_value="2024-01-15"),
        ui.range_calendar(aria_label="Range Calendar"),

        # ── Navigation ──
        ui.link("Example Link", href="https://example.com"),
        ui.breadcrumbs(
            ui.item("Home", key="home"),
            ui.item("Products", key="products"),
            ui.item("Detail", key="detail"),
        ),

        # ── Menus ──
        ui.action_menu("Cut", "Copy", "Paste", on_action=set_menu_action),
        ui.menu_trigger(
            ui.action_button("File Menu"),
            ui.menu(
                ui.item("New", key="new"),
                ui.item("Open", key="open"),
                on_action=set_menu_action,
            ),
        ),

        # ── Disclosure & Accordion ──
        ui.disclosure(
            ui.disclosure_title("Disclosure Title"),
            ui.disclosure_panel("Hidden content here"),
            is_expanded=expanded,
            on_expanded_change=set_expanded,
        ),
        ui.accordion(
            ui.disclosure(
                ui.disclosure_title("Accordion Section 1"),
                ui.disclosure_panel("Accordion content 1"),
            ),
            ui.disclosure(
                ui.disclosure_title("Accordion Section 2"),
                ui.disclosure_panel("Accordion content 2"),
            ),
        ),

        # ── Tabs ──
        ui.tabs(
            ui.tab_list(
                ui.item("Tab A", key="a"),
                ui.item("Tab B", key="b"),
            ),
            ui.tab_panels(
                ui.item(ui.text("Tab A content"), key="a"),
                ui.item(ui.text("Tab B content"), key="b"),
            ),
        ),

        # ── Tags ──
        ui.tag_group(
            [ui.item(t, key=t) for t in tags],
            label="Tags",
            on_remove=lambda keys: set_tags([t for t in tags if t not in keys]),
        ),

        # ── Dialog ──
        ui.dialog_trigger(
            ui.action_button("Open Dialog"),
            ui.dialog(
                ui.heading("Dialog Title"),
                ui.content("Dialog body text"),
                ui.footer("Dialog footer"),
            ),
            is_dismissable=True,
        ),

        # ── Contextual Help ──
        ui.contextual_help(
            ui.heading("Help Title"),
            ui.content("Helpful information"),
        ),

        # ── Alerts ──
        ui.inline_alert(
            ui.heading("Alert"),
            ui.content("Something happened"),
            variant="informative",
        ),

        # ── Illustrated Message ──
        ui.illustrated_message(
            ui.icon("warning"),
            ui.heading("No results"),
            ui.content("Try a different search"),
        ),

        # ── Form ──
        ui.form(
            ui.text_field(label="Form Name"),
            ui.button("Submit", type="submit"),
        ),

        # ── List View ──
        ui.list_view(
            ui.item("Alpha"),
            ui.item("Beta"),
            ui.item("Gamma"),
            aria_label="Sample List",
            selection_mode="MULTIPLE",
        ),

        # ── Table ──
        ui.table(sample_table, front_columns=["Name"]),

        # ── Figure ──
        sample_figure,

        # ── Layout ──
        ui.view(ui.text("Inside view")),
        ui.grid(
            ui.text("Grid A"), ui.text("Grid B"),
            columns=["1fr", "1fr"],
        ),
        ui.fragment(ui.text("Inside fragment")),

        direction="column",
        gap="size-100",
    )


all_components_widget = all_components()
