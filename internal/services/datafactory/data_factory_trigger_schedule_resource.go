package datafactory

import (
	"fmt"
	"log"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/datafactory/mgmt/2018-06-01/datafactory"
	"github.com/Azure/go-autorest/autorest/date"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/azure"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/tf"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/datafactory/parse"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/datafactory/validate"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/suppress"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/validation"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func resourceDataFactoryTriggerSchedule() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Create: resourceDataFactoryTriggerScheduleCreateUpdate,
		Read:   resourceDataFactoryTriggerScheduleRead,
		Update: resourceDataFactoryTriggerScheduleCreateUpdate,
		Delete: resourceDataFactoryTriggerScheduleDelete,

		Importer: pluginsdk.ImporterValidatingResourceId(func(id string) error {
			_, err := parse.TriggerID(id)
			return err
		}),

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(30 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(30 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(30 * time.Minute),
		},

		Schema: map[string]*pluginsdk.Schema{
			"name": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validate.DataFactoryPipelineAndTriggerName(),
			},

			// There's a bug in the Azure API where this is returned in lower-case
			// BUG: https://github.com/Azure/azure-rest-api-specs/issues/5788
			"resource_group_name": azure.SchemaResourceGroupNameDiffSuppress(),

			// TODO remove in 3.0
			"data_factory_name": {
				Type:         pluginsdk.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				ValidateFunc: validate.DataFactoryName(),
				Deprecated:   "`data_factory_name` is deprecated in favour of `data_factory_id` and will be removed in version 3.0 of the AzureRM provider",
				ExactlyOneOf: []string{"data_factory_id"},
			},

			"data_factory_id": {
				Type:         pluginsdk.TypeString,
				Optional:     true, // TODO set to Required in 3.0
				Computed:     true, // TODO remove in 3.0
				ForceNew:     true,
				ValidateFunc: validate.DataFactoryID,
				ExactlyOneOf: []string{"data_factory_name"},
			},

			"description": {
				Type:         pluginsdk.TypeString,
				Optional:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},

			"schedule": {
				Type:     pluginsdk.TypeList,
				Optional: true,
				MinItems: 1,
				MaxItems: 1,
				Elem: &pluginsdk.Resource{
					Schema: map[string]*pluginsdk.Schema{
						"days_of_month": {
							Type:     pluginsdk.TypeList,
							Optional: true,
							Elem: &pluginsdk.Schema{
								Type: pluginsdk.TypeInt,
								ValidateFunc: validation.Any(
									validation.IntBetween(1, 31),
									validation.IntBetween(-31, -1),
								),
							},
						},

						"days_of_week": {
							Type:     pluginsdk.TypeList,
							Optional: true,
							MaxItems: 7,
							Elem: &pluginsdk.Schema{
								Type:         pluginsdk.TypeString,
								ValidateFunc: validation.IsDayOfTheWeek(false),
							},
						},

						"hours": {
							Type:     pluginsdk.TypeList,
							Optional: true,
							Elem: &pluginsdk.Schema{
								Type:         pluginsdk.TypeInt,
								ValidateFunc: validation.IntBetween(0, 24),
							},
						},

						"minutes": {
							Type:     pluginsdk.TypeList,
							Optional: true,
							Elem: &pluginsdk.Schema{
								Type:         pluginsdk.TypeInt,
								ValidateFunc: validation.IntBetween(0, 60),
							},
						},

						"monthly": {
							Type:     pluginsdk.TypeList,
							Optional: true,
							MinItems: 1,
							Elem: &pluginsdk.Resource{
								Schema: map[string]*pluginsdk.Schema{
									"weekday": {
										Type:         pluginsdk.TypeString,
										Required:     true,
										ValidateFunc: validation.IsDayOfTheWeek(false),
									},

									"week": {
										Type:     pluginsdk.TypeInt,
										Optional: true,
										ValidateFunc: validation.Any(
											validation.IntBetween(1, 5),
											validation.IntBetween(-5, -1),
										),
									},
								},
							},
						},
					},
				},
			},

			// This time can only be  represented in UTC.
			// An issue has been filed in the SDK for the timezone attribute that doesn't seem to work
			// https://github.com/Azure/azure-sdk-for-go/issues/6244
			"start_time": {
				Type:             pluginsdk.TypeString,
				Optional:         true,
				Computed:         true,
				DiffSuppressFunc: suppress.RFC3339Time,
				ValidateFunc:     validation.IsRFC3339Time, // times in the past just start immediately
			},

			// This time can only be  represented in UTC.
			// An issue has been filed in the SDK for the timezone attribute that doesn't seem to work
			// https://github.com/Azure/azure-sdk-for-go/issues/6244
			"end_time": {
				Type:             pluginsdk.TypeString,
				Optional:         true,
				DiffSuppressFunc: suppress.RFC3339Time,
				ValidateFunc:     validation.IsRFC3339Time, // times in the past just start immediately
			},

			"frequency": {
				Type:     pluginsdk.TypeString,
				Optional: true,
				Default:  string(datafactory.RecurrenceFrequencyMinute),
				ValidateFunc: validation.StringInSlice([]string{
					string(datafactory.RecurrenceFrequencyMinute),
					string(datafactory.RecurrenceFrequencyHour),
					string(datafactory.RecurrenceFrequencyDay),
					string(datafactory.RecurrenceFrequencyWeek),
					string(datafactory.RecurrenceFrequencyMonth),
				}, false),
			},

			"interval": {
				Type:         pluginsdk.TypeInt,
				Optional:     true,
				Default:      1,
				ValidateFunc: validation.IntAtLeast(1),
			},

			"activated": {
				Type:     pluginsdk.TypeBool,
				Optional: true,
				// Default:  true, // todo 3.0 remove this comment and remove the Computed tag
				Computed: true,
			},

			"pipeline_name": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ValidateFunc: validate.DataFactoryPipelineAndTriggerName(),
			},

			"pipeline_parameters": {
				Type:     pluginsdk.TypeMap,
				Optional: true,
				Elem: &pluginsdk.Schema{
					Type: pluginsdk.TypeString,
				},
			},

			"annotations": {
				Type:     pluginsdk.TypeList,
				Optional: true,
				Elem: &pluginsdk.Schema{
					Type:         pluginsdk.TypeString,
					ValidateFunc: validation.StringIsNotEmpty,
				},
			},
		},
	}
}

func resourceDataFactoryTriggerScheduleCreateUpdate(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).DataFactory.TriggersClient
	subscriptionId := meta.(*clients.Client).Account.SubscriptionId
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	// TODO remove/simplify this after deprecation in 3.0
	var err error
	var dataFactoryId *parse.DataFactoryId
	if v := d.Get("data_factory_name").(string); v != "" {
		newDataFactoryId := parse.NewDataFactoryID(subscriptionId, d.Get("resource_group_name").(string), d.Get("data_factory_name").(string))
		dataFactoryId = &newDataFactoryId
	}
	if v := d.Get("data_factory_id").(string); v != "" {
		dataFactoryId, err = parse.DataFactoryID(v)
		if err != nil {
			return err
		}
	}
	id := parse.NewTriggerID(subscriptionId, dataFactoryId.ResourceGroup, dataFactoryId.FactoryName, d.Get("name").(string))

	if d.IsNewResource() {
		existing, err := client.Get(ctx, id.ResourceGroup, id.FactoryName, id.Name, "")
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("checking for presence of existing %s: %+v", id, err)
			}
		}

		if existing.ID != nil && *existing.ID != "" {
			return tf.ImportAsExistsError("azurerm_data_factory_trigger_schedule", *existing.ID)
		}
	}

	props := &datafactory.ScheduleTriggerTypeProperties{
		Recurrence: &datafactory.ScheduleTriggerRecurrence{
			Frequency: datafactory.RecurrenceFrequency(d.Get("frequency").(string)),
			Interval:  utils.Int32(int32(d.Get("interval").(int))),
			Schedule:  expandDataFactorySchedule(d.Get("schedule").([]interface{})),
		},
	}

	if v, ok := d.GetOk("start_time"); ok {
		t, _ := time.Parse(time.RFC3339, v.(string)) // should be validated by the schema
		props.Recurrence.StartTime = &date.Time{Time: t}
	} else {
		t, _ := time.Parse(time.RFC3339, time.Now().UTC().Format(time.RFC3339))
		props.Recurrence.StartTime = &date.Time{Time: t}
	}

	if v, ok := d.GetOk("end_time"); ok {
		t, _ := time.Parse(time.RFC3339, v.(string)) // should be validated by the schema
		props.Recurrence.EndTime = &date.Time{Time: t}
	}

	reference := &datafactory.PipelineReference{
		ReferenceName: utils.String(d.Get("pipeline_name").(string)),
		Type:          utils.String("PipelineReference"),
	}

	scheduleProps := &datafactory.ScheduleTrigger{
		ScheduleTriggerTypeProperties: props,
		Pipelines: &[]datafactory.TriggerPipelineReference{
			{
				PipelineReference: reference,
				Parameters:        d.Get("pipeline_parameters").(map[string]interface{}),
			},
		},
		Description: utils.String(d.Get("description").(string)),
	}

	if v, ok := d.GetOk("annotations"); ok {
		annotations := v.([]interface{})
		scheduleProps.Annotations = &annotations
	}

	trigger := datafactory.TriggerResource{
		Properties: scheduleProps,
	}

	if _, err := client.CreateOrUpdate(ctx, id.ResourceGroup, id.FactoryName, id.Name, trigger, ""); err != nil {
		return fmt.Errorf("creating %s: %+v", id, err)
	}

	if d.Get("activated").(bool) {
		future, err := client.Start(ctx, id.ResourceGroup, id.FactoryName, id.Name)
		if err != nil {
			return fmt.Errorf("starting %s: %+v", id, err)
		}
		if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
			return fmt.Errorf("waiting on start %s: %+v", id, err)
		}
	}

	d.SetId(id.ID())

	return resourceDataFactoryTriggerScheduleRead(d, meta)
}

func resourceDataFactoryTriggerScheduleRead(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).DataFactory.TriggersClient
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.TriggerID(d.Id())
	if err != nil {
		return err
	}

	dataFactoryId := parse.NewDataFactoryID(id.SubscriptionId, id.ResourceGroup, id.FactoryName)

	resp, err := client.Get(ctx, id.ResourceGroup, id.FactoryName, id.Name, "")
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			d.SetId("")
			log.Printf("[DEBUG] Data Factory Trigger Schedule %q was not found in Resource Group %q - removing from state!", id.Name, id.ResourceGroup)
			return nil
		}
		return fmt.Errorf("retrieving %s: %+v", *id, err)
	}

	d.Set("name", resp.Name)
	d.Set("resource_group_name", id.ResourceGroup)
	// TODO remove in 3.0
	d.Set("data_factory_name", id.FactoryName)
	d.Set("data_factory_id", dataFactoryId.ID())

	scheduleTriggerProps, ok := resp.Properties.AsScheduleTrigger()
	if !ok {
		return fmt.Errorf("classifying Data Factory %s: Expected: %q Received: %q", *id, datafactory.TypeBasicTriggerTypeScheduleTrigger, *resp.Type)
	}

	if scheduleTriggerProps != nil {
		d.Set("activated", scheduleTriggerProps.RuntimeState == datafactory.TriggerRuntimeStateStarted)

		if recurrence := scheduleTriggerProps.Recurrence; recurrence != nil {
			if v := recurrence.StartTime; v != nil {
				d.Set("start_time", v.Format(time.RFC3339))
			}
			if v := recurrence.EndTime; v != nil {
				d.Set("end_time", v.Format(time.RFC3339))
			}
			d.Set("frequency", recurrence.Frequency)
			d.Set("interval", recurrence.Interval)

			if schedule := recurrence.Schedule; schedule != nil {
				d.Set("schedule", flattenDataFactorySchedule(schedule))
			}
		}

		if pipelines := scheduleTriggerProps.Pipelines; pipelines != nil {
			if len(*pipelines) > 0 {
				pipeline := *pipelines
				if reference := pipeline[0].PipelineReference; reference != nil {
					d.Set("pipeline_name", reference.ReferenceName)
				}
				d.Set("pipeline_parameters", pipeline[0].Parameters)
			}
		}

		annotations := flattenDataFactoryAnnotations(scheduleTriggerProps.Annotations)
		if err := d.Set("annotations", annotations); err != nil {
			return fmt.Errorf("setting `annotations`: %+v", err)
		}

		d.Set("description", scheduleTriggerProps.Description)
	}

	return nil
}

func resourceDataFactoryTriggerScheduleDelete(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).DataFactory.TriggersClient
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.TriggerID(d.Id())
	if err != nil {
		return err
	}

	future, err := client.Stop(ctx, id.ResourceGroup, id.FactoryName, id.Name)
	if err != nil {
		return fmt.Errorf("stopping %s: %+v", id, err)
	}
	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("waiting to stop %s: %+v", id, err)
	}

	if _, err = client.Delete(ctx, id.ResourceGroup, id.FactoryName, id.Name); err != nil {
		return fmt.Errorf("deleting %s: %+v", *id, err)
	}

	return nil
}

func expandDataFactorySchedule(input []interface{}) *datafactory.RecurrenceSchedule {
	if len(input) == 0 || input[0] == nil {
		return nil
	}

	schedule := datafactory.RecurrenceSchedule{}

	value := input[0].(map[string]interface{})
	weekDays := make([]datafactory.DaysOfWeek, 0)
	for _, v := range value["days_of_week"].([]interface{}) {
		weekDays = append(weekDays, datafactory.DaysOfWeek(v.(string)))
	}
	if len(weekDays) > 0 {
		schedule.WeekDays = &weekDays
	}

	monthlyOccurrences := make([]datafactory.RecurrenceScheduleOccurrence, 0)
	for _, v := range value["monthly"].([]interface{}) {
		value := v.(map[string]interface{})
		monthlyOccurrences = append(monthlyOccurrences, datafactory.RecurrenceScheduleOccurrence{
			Day:        datafactory.DayOfWeek(value["weekday"].(string)),
			Occurrence: utils.Int32(int32(value["week"].(int))),
		})
	}
	if len(monthlyOccurrences) > 0 {
		schedule.MonthlyOccurrences = &monthlyOccurrences
	}

	if monthdays := value["days_of_month"].([]interface{}); len(monthdays) > 0 {
		schedule.MonthDays = utils.ExpandInt32Slice(monthdays)
	}
	if minutes := value["minutes"].([]interface{}); len(minutes) > 0 {
		schedule.Minutes = utils.ExpandInt32Slice(minutes)
	}
	if hours := value["hours"].([]interface{}); len(hours) > 0 {
		schedule.Hours = utils.ExpandInt32Slice(hours)
	}

	return &schedule
}

func flattenDataFactorySchedule(schedule *datafactory.RecurrenceSchedule) []interface{} {
	if schedule == nil {
		return []interface{}{}
	}
	value := make(map[string]interface{})
	if schedule.Minutes != nil {
		value["minutes"] = utils.FlattenInt32Slice(schedule.Minutes)
	}
	if schedule.Hours != nil {
		value["hours"] = utils.FlattenInt32Slice(schedule.Hours)
	}
	if schedule.WeekDays != nil {
		weekDays := make([]interface{}, 0)
		for _, v := range *schedule.WeekDays {
			weekDays = append(weekDays, string(v))
		}
		value["days_of_week"] = weekDays
	}
	if schedule.MonthDays != nil {
		value["days_of_month"] = utils.FlattenInt32Slice(schedule.MonthDays)
	}
	if schedule.MonthlyOccurrences != nil {
		monthlyOccurrences := make([]interface{}, 0)
		for _, v := range *schedule.MonthlyOccurrences {
			occurrence := make(map[string]interface{})
			occurrence["weekday"] = string(v.Day)
			if v.Occurrence != nil {
				occurrence["week"] = *v.Occurrence
			}
			monthlyOccurrences = append(monthlyOccurrences, occurrence)
		}
		value["monthly"] = monthlyOccurrences
	}
	return []interface{}{value}
}
