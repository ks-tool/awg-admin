/*
  Copyright © 2026 Alexey Shulutkov <github@shulutkov.ru>

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

  	http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
*/

import type {ReactNode} from 'react';
import Form, {type IChangeEvent} from '@rjsf/core';
import validator from '@rjsf/validator-ajv8';
import {TranslatableString, type IconButtonProps, type RJSFSchema, type UiSchema} from '@rjsf/utils';
import {ArrowDown, ArrowUp, Copy, Plus, Trash2, X} from 'lucide-react';
import {buttons} from '@/components/common/controls';

function arrayButton(props: IconButtonProps, label: TranslatableString, icon: ReactNode) {
    return (
        <button
            type="button"
            id={props.id}
            className={`btn ${label === TranslatableString.RemoveButton ? 'btn-danger' : ''} ${props.className ?? ''}`}
            onClick={props.onClick}
            disabled={props.disabled}
            title={props.registry.translateString(label)}
        >
            {icon}
        </button>
    );
}

const buttonTemplates = {
    AddButton: (props: IconButtonProps) => <div className="flex justify-end">{arrayButton(props, TranslatableString.AddButton, <Plus size={16} />)}</div>,
    CopyButton: (props: IconButtonProps) => arrayButton(props, TranslatableString.CopyButton, <Copy size={16} />),
    MoveUpButton: (props: IconButtonProps) => arrayButton(props, TranslatableString.MoveUpButton, <ArrowUp size={16} />),
    MoveDownButton: (props: IconButtonProps) => arrayButton(props, TranslatableString.MoveDownButton, <ArrowDown size={16} />),
    RemoveButton: (props: IconButtonProps) => arrayButton(props, TranslatableString.RemoveButton, <Trash2 size={16} />),
    ClearButton: (props: IconButtonProps) => arrayButton(props, TranslatableString.ClearButton, <X size={16} />),
};

interface SchemaFormProps {
    schema: Record<string, unknown>;
    uiSchema?: Record<string, unknown>;
    formData: Record<string, unknown>;
    submitLabel: string;
    disabled?: boolean;
    onChange?: (value: Record<string, unknown>) => void;
    onSubmit: (value: Record<string, unknown>) => void | Promise<void>;
}

export function SchemaForm({schema, uiSchema, formData, submitLabel, disabled, onChange, onSubmit}: SchemaFormProps) {
    const handleChange = (event: IChangeEvent) => {
        onChange?.(event.formData as Record<string, unknown>);
    };
    const handleSubmit = (event: IChangeEvent) => {
        void onSubmit(event.formData as Record<string, unknown>);
    };

    return (
        <div className="rjsf-form">
            <Form
                schema={schema as RJSFSchema}
                uiSchema={uiSchema as UiSchema}
                formData={formData}
                validator={validator}
                disabled={disabled}
                templates={{ButtonTemplates: buttonTemplates}}
                onChange={handleChange}
                onSubmit={handleSubmit}
            >
                <button type="submit" disabled={disabled} className={`${buttons.primary} mt-4 w-full`}>
                    {submitLabel}
                </button>
            </Form>
        </div>
    );
}
