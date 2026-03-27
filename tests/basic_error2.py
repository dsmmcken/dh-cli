from deephaven import read_csv, merge, ui, agg
from deephaven.plot import express as dx

# Load both CSV files
math_students = read_csv("evals/data/uciml--student-alcohol-consumption/student-mat.csv")
por_students = read_csv("evals/data/uciml--student-alcohol-consumption/student-por.csv")

# Add a Course column to distinguish them, then merge (union)
math_with_course = math_students.update(["Course = `Math`"])
por_with_course = por_students.update(["Course = `Portuguese`"])

# Merge (combine) both tables into one
all_students = merge([math_with_course, por_with_course])

# Add a computed TotalAlcohol column (average of weekday and weekend)
all_students = all_students.update_view(
    ["TotalAlcohol = (Dalc + Walc) / 2.0"]
)

# Average grades by weekday alcohol level (Dalc), grouped by Course
avg_by_dalc = all_students.agg_by(
    [
        agg.avg(cols=["AvgG3 = G3"]),
        agg.count_(col="Count"),
    ],
    by=["Course", "Dalc"],
)

# Average grades by weekend alcohol level (Walc), grouped by Course
avg_by_walc = all_students.agg_by(
    [
        agg.avg(cols=["AvgG3 = G3"]),
        agg.count_(col="Count"),
    ],
    by=["Course", "Walc"],
)


@ui.component
def layout():
    course, set_course = ui.use_state("Math")

    # Filter tables by selected course
    filtered = ui.use_memo(
        lambda: all_students.where(f"Course = `{course}`"), [course]
    )

    filtered_avg_dalc = ui.use_memo(
        lambda: avg_by_dalc.where(f"Course = `{course}`"), [course]
    )

    filtered_avg_walc = ui.use_memo(
        lambda: avg_by_walc.where(f"Course = `{course}`"), [course]
    )

    # Scatter plot: TotalAlcohol vs G3 (final grade)
    scatter = ui.use_memo(
        lambda: dx.scatter(
            filtered,
            x="TotalAlcohol",
            y="G3",
            title=f"Alcohol Consumption vs Final Grade ({course})",
            xaxis_titles="Alcohol (avg of weekday + weekend)",
            yaxis_titles="Final Grade (G3)",
        ),
        [filtered],
    )

    # Bar chart: average G3 by weekday alcohol level (Dalc)
    bar_dalc = ui.use_memo(
        lambda: dx.bar(
            filtered_avg_dalc,
            x="Dalc",
            y="AvgG3",
            title=f"Avg Final Grade by Weekday Alcohol Level ({course})",
            xaxis_titles="Weekday Alcohol (Dalc)",
            yaxis_titles="Average Final Grade",
        ),
        [filtered_avg_dalc],
    )

    # Bar chart: average G3 by weekend alcohol level (Walc)
    bar_walc = ui.use_memo(
        lambda: dx.bar(
            filtered_avg_walc,
            x="Walc",
            y="AvgG3",
            title=f"Avg Final Grade by Weekend Alcohol Level ({course})",
            xaxis_titles="Weekend Alcohol (Walc)",
            yaxis_titles="Average Final Grade",
        ),
        [filtered_avg_walc],
    )

    return ui.column(
        ui.row(
            ui.panel(
                ui.picker(
                    "Math",
                    "Portuguese",
                    label="Select Course",
                    selected_key=course,
                    on_change=set_course,
                ),
                title="Controls",
            ),
            height=10,
        ),
        ui.row(
            ui.panel(scatter, title="Alcohol vs Grade (Scatter)"),
            ui.panel(ui.table(filtered), title="Student Data"),
            height=45,
        ),
        ui.row(
            ui.panel(bar_dalc, title="Avg Grade by Weekday Alcohol"),
            ui.panel(bar_walc, title="Avg Grade by Weekend Alcohol"),
            height=45,
        ),
    )


dashboard = ui.dashboard(layout())