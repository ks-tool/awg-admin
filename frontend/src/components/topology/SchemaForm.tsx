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

import Form, {type IChangeEvent} from '@rjsf/core';
import validator from '@rjsf/validator-ajv8';
import type {RJSFSchema, UiSchema} from '@rjsf/utils';
import {buttons} from '@/components/common/controls';

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
