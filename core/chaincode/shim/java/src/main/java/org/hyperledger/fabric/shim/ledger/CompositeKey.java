/*
Copyright IBM 2017 All Rights Reserved.

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
package org.hyperledger.fabric.shim.ledger;

import static java.util.stream.Collectors.joining;

import java.util.Arrays;
import java.util.Collections;
import java.util.List;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

public class CompositeKey {

	private static final String DELIMITER = new String(Character.toChars(Character.MIN_CODE_POINT));
	private static final String INVALID_SEGMENT_CHAR = new String(Character.toChars(Character.MAX_CODE_POINT));
	private static final String INVALID_SEGMENT_PATTERN = String.format("(?:%s|%s)", INVALID_SEGMENT_CHAR, DELIMITER);

	final String objectType;
	final List<String> attributes;
	final String compositeKey;

	public CompositeKey(String objectType, String... attributes) {
		this(objectType, attributes == null ? Collections.emptyList() : Arrays.asList(attributes));
	}

	public CompositeKey(String objectType, List<String> attributes) {
		if (objectType == null) throw new NullPointerException("objectType cannot be null");
		this.objectType = objectType;
		this.attributes = attributes;
		this.compositeKey = generateCompositeKeyString(objectType, attributes);
	}

	public String getObjectType() {
		return objectType;
	}

	public List<String> getAttributes() {
		return attributes;
	}

	@Override
	public String toString() {
		return compositeKey;
	}

	public static CompositeKey parseCompositeKey(String compositeKey) {
		if (compositeKey == null) return null;
		final String[] segments = compositeKey.split(DELIMITER, 0);
		return new CompositeKey(segments[0], Arrays.stream(segments).skip(1).toArray(String[]::new));
	}

	private String generateCompositeKeyString(String objectType, List<String> attributes) {

		// object type must be a valid composite key segment
		validateCompositeKeySegment(objectType);

		// the attributes must be valid composite key segments
		attributes.stream().forEach(x -> validateCompositeKeySegment(x));

		// return objectType + DELIMITER + (attribute + DELIMITER)*
		return attributes.stream().collect(joining(DELIMITER, objectType + DELIMITER, DELIMITER));

	}

	private void validateCompositeKeySegment(String segment) {
		final Matcher matcher = Pattern.compile(INVALID_SEGMENT_PATTERN).matcher(segment);
		if (matcher.find()) {
			throw CompositeKeyFormatException.forInputString(segment, matcher.group(), matcher.start());
		}
	}

}
