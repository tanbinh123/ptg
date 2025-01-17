package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/crossoverJie/ptg/gui/call"
	"github.com/crossoverJie/ptg/gui/io"
	"github.com/crossoverJie/ptg/reflect"
	_ "github.com/crossoverJie/ptg/reflect"
	"github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"image/color"
	gio "io"
	"net/url"
	"strings"
)

func main() {
	ptgApp := InitApp()
	app := app.New()
	window := app.NewWindow(ptgApp.AppName)
	window.Resize(fyne.NewSize(ptgApp.AppWidth, ptgApp.AppHeight))

	requestEntry := widget.NewMultiLineEntry()
	requestEntry.SetPlaceHolder(ptgApp.RightRequest.RequestEntryPlaceHolder)
	requestEntry.Wrapping = fyne.TextWrapWord
	responseEntry := widget.NewMultiLineEntry()
	responseEntry.Wrapping = fyne.TextWrapWord
	reqLabel := widget.NewLabel("")
	targetInput := widget.NewEntry()
	targetInput.SetText(ptgApp.RightRequest.TargetInputText)
	targetInput.SetPlaceHolder("")
	metadataEntry := widget.NewMultiLineEntry()
	metadataEntry.SetPlaceHolder(ptgApp.RightRequest.MetaDataInputPlaceHolder)
	processBar := widget.NewProgressBarInfinite()
	processBar.Hide()
	serviceAccordionRemove := false
	serviceAccordion := widget.NewAccordion()
	searchAccordion := widget.NewAccordion()
	searchEntry := widget.NewEntry()
	historyButton := container.NewVBox()
	history := NewHistory(10, historyButton, targetInput, requestEntry, metadataEntry, responseEntry, reqLabel)
	historyId := 0

	content := container.NewVBox()
	newProto := func(uri fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, window)
			return
		}
		if uri == nil {
			return
		}

		parseAdapter, exit, err := RegisterReflect(uri.URI().Path())
		if err != nil {
			dialog.ShowError(err, window)
			return
		}
		if exit {
			dialog.ShowError(errors.New("proto file already exists"), window)
			return
		}

		maps := parseAdapter.Parse().ServiceInfoMaps()
		if serviceAccordionRemove {
			content.Add(serviceAccordion)
			serviceAccordionRemove = false
		}
		for k, v := range maps {
			var methods []string
			for _, s := range v {
				methods = append(methods, k+"."+s+"-"+fmt.Sprint(parseAdapter.Index()))
			}
			serviceAccordion.Append(&widget.AccordionItem{
				Title: k,
				Detail: widget.NewRadioGroup(methods, func(s string) {
					if s == "" {
						return
					}
					methodInfo := strings.Split(s, "-")
					service, method, err := reflect.ParseServiceMethod(methodInfo[0])
					if err != nil {
						dialog.ShowError(err, window)
						return
					}
					adapter, err := GetParseAdapter(methodInfo[1])
					if err != nil {
						dialog.ShowError(err, window)
						return
					}
					json, err := adapter.Parse().RequestJSON(service, method)
					if err != nil {
						dialog.ShowError(err, window)
						return
					}
					requestEntry.SetText(json)
					reqLabel.SetText(s)

				}),
				Open: false,
			})
		}

	}
	fileOpen := dialog.NewFileOpen(newProto, window)

	toolbar := widget.NewToolbar(
		widget.NewToolbarAction(theme.ContentAddIcon(), func() {
			fileOpen.Show()
		}),
		widget.NewToolbarAction(theme.ViewRefreshIcon(), func() {
			content.Remove(serviceAccordion)
			serviceAccordionRemove = true
			serviceAccordion.Items = nil
			dialog.ShowInformation("Notice", "Reload success.", window)
			ReloadReflect(newProto)
		}),
		widget.NewToolbarAction(theme.DeleteIcon(), func() {
			ClearReflect()
			content.Remove(serviceAccordion)
			serviceAccordionRemove = true
			serviceAccordion.Items = nil
			dialog.ShowInformation("Notice", "All proto files have been reset", window)
		}),
		widget.NewToolbarSeparator(),
		widget.NewToolbarAction(theme.HelpIcon(), func() {
			w := fyne.CurrentApp().NewWindow("Help")
			u, _ := url.Parse(ptgApp.HelpUrl)
			w.SetContent(container.New(layout.NewCenterLayout(), widget.NewHyperlink("help?", u)))
			w.Resize(fyne.NewSize(130, 100))
			w.SetFixedSize(true)
			w.Show()
		}),
		//widget.NewToolbarAction(theme.RadioButtonIcon(), func() {
		//	w := fyne.CurrentApp().NewWindow("Performance test")
		//	w.Resize(fyne.NewSize(ptgApp.AppWidth, ptgApp.AppHeight))
		//
		//	myCanvas := w.Canvas()
		//	//blue := color.NRGBA{R: 0, G: 0, B: 180, A: 255}
		//	//rect := canvas.NewRectangle(blue)
		//	//myCanvas.SetContent(rect)
		//
		//	red := color.NRGBA{R: 0xff, G: 0x33, B: 0x33, A: 0xff}
		//	circle := canvas.NewCircle(color.White)
		//	circle.StrokeWidth = 4
		//	circle.StrokeColor = red
		//
		//	line := canvas.NewLine(red)
		//	myCanvas.SetContent(line)
		//
		//	w.SetFixedSize(true)
		//	w.Show()
		//}),
	)
	content.Add(toolbar)
	content.Add(searchAccordion)
	content.Add(serviceAccordion)

	// Search
	searchEntry.SetPlaceHolder(ptgApp.SearchFormPlaceHolder)
	searchForm := widget.NewForm(&widget.FormItem{
		Widget:   searchEntry,
		HintText: ptgApp.SearchFormText,
	}, &widget.FormItem{
		Widget: widget.NewButtonWithIcon(ptgApp.SearchFormText, theme.SearchIcon(), func() {
			history.SearchResult(strings.ToLower(searchEntry.Text))
		}),
	})

	searchList := container.NewVScroll(historyButton)
	searchAccordion.Append(widget.NewAccordionItem(ptgApp.SearchFormText, searchForm))
	leftTool := container.New(layout.NewGridLayout(1), content, searchList)

	// Right
	form := widget.NewForm(&widget.FormItem{
		Text:     ptgApp.RightRequest.TargetFormText,
		Widget:   targetInput,
		HintText: ptgApp.RightRequest.TargetFormHintText,
	})

	requestContainer := container.New(layout.NewGridLayoutWithColumns(1))
	requestContainer.Add(requestEntry)
	requestButton := widget.NewButtonWithIcon("RUN", theme.MediaPlayIcon(), func() {
		if requestEntry.Text == "" {
			dialog.ShowError(errors.New("request json can not nil"), window)
			return
		}
		if reqLabel.Text == "" {
			dialog.ShowError(errors.New("proto can not nil"), window)
			return
		}
		methodInfo := strings.Split(reqLabel.Text, "-")
		service, method, err := reflect.ParseServiceMethod(methodInfo[0])
		if err != nil {
			dialog.ShowError(err, window)
			return
		}
		index := methodInfo[1]
		adapter, err := GetParseAdapter(index)
		if err != nil {
			dialog.ShowError(err, window)
			return
		}
		parse := adapter.Parse()
		mds, err := parse.MethodDescriptor(service, method)
		if err != nil {
			dialog.ShowError(err, window)
			return
		}
		var opts []grpc.DialOption
		opts = append(opts, grpc.WithInsecure())
		ctx := context.Background()
		ctx, err = buildWithMetadata(ctx, metadataEntry.Text)
		if err != nil {
			dialog.ShowError(err, window)
			return
		}

		conn, err := grpc.DialContext(ctx, targetInput.Text, opts...)
		if err != nil {
			dialog.ShowError(err, window)
			return
		}
		stub := grpcdynamic.NewStub(conn)
		processBar.Show()
		// call
		callBuilder := call.NewCallBuilder().Parse(parse).
			ResponseEntry(responseEntry).
			RequestEntry(requestEntry).
			Mds(mds).
			Stub(stub).
			RequestEntry(requestEntry).
			ProcessBar(processBar).
			ErrorHandle(func(window fyne.Window, processBar *widget.ProgressBarInfinite, err error) {
				processBar.Hide()
				dialog.ShowError(err, window)
			})
		response, err := callBuilder.Run(ctx)
		if err != nil {
			processBar.Hide()
			dialog.ShowError(err, window)
			return
		}
		processBar.Hide()

		// Write history
		historyId++
		history.Put(historyId, &io.SearchLog{
			Id: int32(historyId),
			Value: &io.Log{
				Target:   targetInput.Text,
				Request:  requestEntry.Text,
				Metadata: metadataEntry.Text,
				Response: response,
			},
			MethodInfo: reqLabel.Text},
		)

	})
	bottomBox := container.NewVBox(widget.NewAccordion(&widget.AccordionItem{
		Title:  ptgApp.RightRequest.MetaDataAccordionTitle,
		Detail: metadataEntry,
		Open:   false,
	}))
	bottomBox.Add(canvas.NewLine(color.Black))
	bottomBox.Add(requestButton)
	bottomBox.Add(canvas.NewLine(color.Black))
	bottomBox.Add(processBar)
	requestPanel := container.NewBorder(form, bottomBox, nil, nil)
	requestPanel.Add(requestContainer)

	responseContainer := container.New(layout.NewGridLayoutWithColumns(1))
	responseContainer.Add(responseEntry)
	responseLabel := widget.NewLabel(ptgApp.RightResponse.ResponseLabelText)
	responsePanel := container.NewBorder(responseLabel, nil, nil, nil)
	responsePanel.Add(responseContainer)

	rightTool := container.NewGridWithColumns(1,
		requestPanel, responsePanel)
	split := container.NewHSplit(leftTool, rightTool)

	window.SetContent(split)
	app.Lifecycle().SetOnStarted(func() {
		log, err := io.LoadLogWithStruct()
		if err != nil {
			dialog.ShowError(err, window)
			return
		}
		for _, filename := range log.Filenames {
			newProto(&ResetUri{
				Filename: filename,
			}, nil)
		}
		if log.Target != "" {
			targetInput.SetText(log.Target)
		}
		if log.Request != "" {
			requestEntry.SetText(log.Request)
		}
		if log.Response != "" {
			responseEntry.SetText(log.Response)
		}
		if log.Metadata != "" {
			metadataEntry.SetText(log.Metadata)
		}
		searchLog, err := io.LoadSearchLogWithStruct()
		if err != nil {
			dialog.ShowError(err, window)
			return
		}
		history.InitSearchLog(searchLog)

	})
	app.Lifecycle().SetOnStopped(func() {
		var filenames []string
		for filename, _ := range ParseContainer() {
			filenames = append(filenames, filename)
		}
		err := SaveLog(filenames, targetInput.Text, requestEntry.Text, responseEntry.Text, metadataEntry.Text)
		if err != nil {
			dialog.ShowError(err, window)
			return
		}
		err = history.SaveLog()
		if err != nil {
			dialog.ShowError(err, window)
			return
		}
	})
	window.ShowAndRun()
}

func buildWithMetadata(ctx context.Context, meta string) (context.Context, error) {
	if strings.Trim(meta, "") != "" {
		var m map[string]string
		err := json.Unmarshal([]byte(meta), &m)
		if err != nil {
			return nil, err
		}
		md := metadata.New(m)
		ctx := metadata.NewOutgoingContext(ctx, md)
		return ctx, nil
	}
	return ctx, nil

}

func SaveLog(filenames []string, target, request, response, metadata string) error {
	log := io.Log{
		Filenames: filenames,
		Target:    target,
		Request:   request,
		Metadata:  metadata,
		Response:  response,
	}
	marshal, err := proto.Marshal(&log)
	if err != nil {
		return err
	}
	return io.SaveLog(io.AppLog, marshal)
}

type ResetUri struct {
	gio.ReadCloser
	Filename string
}

func (r *ResetUri) URI() fyne.URI {
	return &uri{path: r.Filename}
}

type uri struct {
	path string
}

func (u *uri) Extension() string {
	return ""
}

func (u *uri) Name() string {
	return ""
}

func (u *uri) MimeType() string {
	return ""
}

func (u *uri) Scheme() string {
	return ""
}

func (u *uri) String() string {
	return ""
}

func (u *uri) Authority() string {
	return ""
}

func (u *uri) Path() string {
	return u.path
}

func (u *uri) Query() string {
	return ""
}

func (u *uri) Fragment() string {
	return ""
}
